module Dapp
  module Kube
    module Kubernetes::Manager
      class Deployment < Base
        # NOTICE: @revision_before_deploy на данный момент выводится на экран как информация для дебага.
        # NOTICE: deployment.kubernetes.io/revision не меняется при изменении количества реплик, поэтому
        # NOTICE: критерий ожидания по изменению ревизии не верен.
        # NOTICE: Однако, при обновлении deployment ревизия сбрасывается и ожидание переустановки этой ревизии
        # NOTICE: является одним из критериев завершения ожидания на данный момент.

        def before_deploy
          if dapp.kubernetes.deployment? name
            d = Kubernetes::Client::Resource::Deployment.new(dapp.kubernetes.deployment(name))

            @revision_before_deploy = d.annotations['deployment.kubernetes.io/revision']

            unless @revision_before_deploy.nil?
              new_spec = Marshal.load(Marshal.dump(d.spec))
              new_spec.delete('status')
              new_spec.fetch('metadata', {}).fetch('annotations', {}).delete('deployment.kubernetes.io/revision')

              @deployment_before_deploy = Kubernetes::Client::Resource::Deployment.new(dapp.kubernetes.replace_deployment!(name, new_spec))
            end
          end

          @deploy_began_at = Time.now
        end

        def after_deploy
          @deployed_at = Time.now
        end

        def should_watch?
          d = Kubernetes::Client::Resource::Deployment.new(dapp.kubernetes.deployment(name))

          ["dapp/watch", "dapp/watch-logs"].all? do |anno|
            d.annotations[anno] != "false"
          end
        end

        def watch_till_ready!
          dapp.log_process("Watch deployment '#{name}' till ready") do
            known_events_by_pod = {}
            known_log_timestamps_by_pod_and_container = {}

            d = @deployment_before_deploy || Kubernetes::Client::Resource::Deployment.new(dapp.kubernetes.deployment(name))

            loop do
              d_revision = d.annotations['deployment.kubernetes.io/revision']

              dapp.log_step("[#{Time.now}] Poll deployment '#{d.name}' status")
              dapp.with_log_indent do
                dapp.log_info("Replicas: #{_field_value_for_log(d.status['replicas'])}")
                dapp.log_info("Updated replicas: #{_field_value_for_log(d.status['updatedReplicas'])}")
                dapp.log_info("Available replicas: #{_field_value_for_log(d.status['availableReplicas'])}")
                dapp.log_info("Unavailable replicas: #{_field_value_for_log(d.status['unavailableReplicas'])}")
                dapp.log_info("Ready replicas: #{_field_value_for_log(d.status['readyReplicas'])} / #{_field_value_for_log(d.replicas)}")
              end

              rs = nil
              if d_revision
                # Находим актуальный, текущий ReplicaSet.
                # Если такая ситуация, когда есть несколько подходящих по revision ReplicaSet, то берем старейший по дате создания.
                # Также делает kubectl: https://github.com/kubernetes/kubernetes/blob/d86a01570ba243e8d75057415113a0ff4d68c96b/pkg/controller/deployment/util/deployment_util.go#L664
                rs = dapp.kubernetes.replicaset_list['items']
                  .map {|spec| Kubernetes::Client::Resource::Replicaset.new(spec)}
                  .select do |_rs|
                    Array(_rs.metadata['ownerReferences']).any? do |owner_reference|
                      owner_reference['uid'] == d.metadata['uid']
                    end
                  end
                  .select do |_rs|
                    rs_revision = _rs.annotations['deployment.kubernetes.io/revision']
                    (rs_revision and (d_revision == rs_revision))
                  end
                  .sort_by do |_rs|
                    if creation_timestamp = _rs.metadata['creationTimestamp']
                      Time.parse(creation_timestamp)
                    else
                      Time.now
                    end
                  end.first
              end

              if rs
                dapp.with_log_indent do
                  dapp.log_step("Current ReplicaSet '#{rs.name}' status")
                  dapp.with_log_indent do
                    dapp.log_info("Replicas: #{_field_value_for_log(rs.status['replicas'])}")
                    dapp.log_info("Fully labeled replicas: #{_field_value_for_log(rs.status['fullyLabeledReplicas'])}")
                    dapp.log_info("Available replicas: #{_field_value_for_log(rs.status['availableReplicas'])}")
                    dapp.log_info("Ready replicas: #{_field_value_for_log(rs.status['readyReplicas'])} / #{_field_value_for_log(d.replicas)}")
                  end
                end

                # Pod'ы связанные с активным ReplicaSet
                rs_pods = dapp.kubernetes.pod_list['items']
                  .map {|spec| Kubernetes::Client::Resource::Pod.new(spec)}
                  .select do |pod|
                    Array(pod.metadata['ownerReferences']).any? do |owner_reference|
                      owner_reference['uid'] == rs.metadata['uid']
                    end
                  end

                dapp.with_log_indent do
                  dapp.log_step("Pods:") if rs_pods.any?

                  rs_pods.each do |pod|
                    dapp.with_log_indent do
                      dapp.log_step(pod.name)

                      known_events_by_pod[pod.name] ||= []
                      pod_events = dapp.kubernetes
                        .event_list(fieldSelector: "involvedObject.uid=#{pod.uid}")['items']
                        .map {|spec| Kubernetes::Client::Resource::Event.new(spec)}
                        .reject do |event|
                          known_events_by_pod[pod.name].include? event.uid
                        end

                      if pod_events.any?
                        dapp.with_log_indent do
                          dapp.log_step("Last events:")
                          pod_events.each do |event|
                            dapp.with_log_indent do
                              dapp.log_info("[#{event.metadata['creationTimestamp']}] #{event.spec['message']}")
                            end
                            known_events_by_pod[pod.name] << event.uid
                          end
                        end
                      end

                      dapp.with_log_indent do
                        pod.containers_names.each do |container_name|
                          known_log_timestamps_by_pod_and_container[pod.name] ||= {}
                          known_log_timestamps_by_pod_and_container[pod.name][container_name] ||= Set.new

                          since_time = nil
                          # Если под еще не перешел в состоянии готовности, то можно вывести все логи которые имеются.
                          # Иначе выводим только новые логи с момента начала текущей сессии деплоя.
                          if [nil, "True"].include? pod.ready_condition_status
                            since_time = @deploy_began_at.utc.iso8601(9) if @deploy_began_at
                          end

                          log_lines_by_time = []
                          begin
                            log_lines_by_time = dapp.kubernetes.pod_log(pod.name, container: container_name, timestamps: true, sinceTime: since_time)
                              .lines.map(&:strip)
                              .map {|line|
                                timestamp, _, data = line.partition(' ')
                                unless known_log_timestamps_by_pod_and_container[pod.name][container_name].include? timestamp
                                  known_log_timestamps_by_pod_and_container[pod.name][container_name].add timestamp
                                  [timestamp, data]
                                end
                              }.compact
                          rescue Kubernetes::Client::Error::Pod::ContainerCreating, Kubernetes::Client::Error::Pod::PodInitializing
                            next
                          rescue Kubernetes::Client::Error::Base => err
                            dapp.log_warning("#{dapp.log_time}Error while fetching pod's #{pod.name} logs: #{err.message}", stream: dapp.service_stream)
                            next
                          end

                          if log_lines_by_time.any?
                            dapp.log_step("Last container '#{container_name}' log:")
                            dapp.with_log_indent do
                              log_lines_by_time.each do |timestamp, line|
                                dapp.log("[#{timestamp}] #{line}")
                              end
                            end
                          end
                        end
                      end

                      pod_manager = Kubernetes::Manager::Pod.new(dapp, pod.name)
                      pod_manager.check_readiness_condition_if_available!(pod)
                    end # with_log_indent
                  end # rs_pods.each
                end # with_log_indent
              end

              # break only when rs is not nil

              if d_revision && d.replicas && d.replicas == 0
                break
              end

              if d_revision && d.replicas && rs
                break if is_deployment_ready(d) && is_replicaset_ready(d, rs)
              end

              sleep 5
              d = Kubernetes::Client::Resource::Deployment.new(dapp.kubernetes.deployment(d.name))
            end
          end
        end

        def is_deployment_ready(d)
          d.status.key?("readyReplicas") && d.status["readyReplicas"] >= d.replicas
        end

        def is_replicaset_ready(d, rs)
          rs.status.key?("readyReplicas") && rs.status["readyReplicas"] >= d.replicas
        end

        private

        def _field_value_for_log(value)
          value ? value : '-'
        end
      end # Deployment
    end # Kubernetes::Manager
  end # Kube
end # Dapp
