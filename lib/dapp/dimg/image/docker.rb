module Dapp
  module Dimg
    module Image
      class Docker
        attr_reader :from
        attr_reader :name
        attr_reader :dapp

        def self.image_by_name(name:, **kwargs)
          (@images ||= {})[name] ||= new(name: name, **kwargs)
        end

        def initialize(name:, dapp:, from: nil)
          @from = from
          @name = name
          @dapp = dapp
        end

        def id
          cache[:id]
        end

        def untag!
          raise Error::Build, code: :image_already_untagged, data: { name: name } unless tagged?
          dapp.shellout!("#{dapp.host_docker} rmi #{name}")
          self.cache.delete(name)
        end

        def push!
          raise Error::Build, code: :image_not_exist, data: { name: name } unless tagged?
          dapp.log_secondary_process(dapp.t(code: 'process.image_push', data: { name: name })) do
            dapp.shellout!("#{dapp.host_docker} push #{name}", verbose: true)
          end
        end

        def pull!
          return if tagged?
          dapp.log_secondary_process(dapp.t(code: 'process.image_pull', data: { name: name })) do
            dapp.shellout!("#{dapp.host_docker} pull #{name}", verbose: true)
          end
          cache_reset
        end

        def tagged?
          !!id
        end

        def created_at
          raise Error::Build, code: :image_not_exist, data: { name: name } unless tagged?
          cache[:created_at]
        end

        def size
          raise Error::Build, code: :image_not_exist, data: { name: name } unless tagged?
          cache[:size]
        end

        def self.image_config_option(image_id:, option:)
          output = ::Dapp::Dapp.shellout!("#{::Dapp::Dapp.host_docker} inspect --format='{{json .Config.#{option.to_s.capitalize}}}' #{image_id}").stdout.strip
          output == 'null' ? [] : JSON.parse(output)
        end

        def cache_reset
          self.class.cache_reset(name)
        end

        protected

        def cache
          self.class.cache[name.to_s] || {}
        end

        class << self
          def image_name_format
            "#{DockerRegistry.repo_name_format}(:(?<tag>#{tag_format}))?"
          end

          def tag_format
            '(?![-.])[a-zA-Z0-9_.-]{1,127}'
          end

          def image_name?(name)
            !(/^#{image_name_format}$/ =~ name).nil?
          end

          def tag?(name)
            !(/^#{tag_format}$/ =~ name).nil?
          end

          def tag!(id:, tag:, verbose: false, quiet: false)
            ::Dapp::Dapp.shellout!("#{::Dapp::Dapp.host_docker} tag #{id} #{tag}", verbose: verbose, quiet: quiet)

            if cache_entry = cache.values.find {|v| v[:id] == id}
              cache[tag] = cache_entry
            else
              cache_reset
            end
          end

          def save!(image_or_images, file_path, verbose: false, quiet: false)
            images = Array(image_or_images).join(' ')
            ::Dapp::Dapp.shellout!("#{::Dapp::Dapp.host_docker} save -o #{file_path} #{images}", verbose: verbose, quiet: quiet)
          end

          def load!(file_path, verbose: false, quiet: false)
            ::Dapp::Dapp.shellout!("#{::Dapp::Dapp.host_docker} load -i #{file_path}", verbose: verbose, quiet: quiet)
          end

          def cache
            @cache ||= (@cache = {}).tap { cache_reset }
          end

          def cache_reset(name = '')
            # FIXME: rework images cache, then delete time measure
            #t = Time.now
            cache.delete(name)
            ::Dapp::Dapp.shellout!("#{::Dapp::Dapp.host_docker} images --format='{{.Repository}}:{{.Tag}};{{.ID}};{{.CreatedAt}};{{.Size}}' --no-trunc #{name}")
                        .stdout
                        .lines
                        .each do |l|
              name, id, created_at, size_field = l.split(';').map(&:strip)
              name = name.reverse.chomp('docker.io/'.reverse).reverse
              size = begin
                match = size_field.match(/^(\S*\d)\ ?(b|kb|mb|gb|tb)$/i)
                raise Error::Build, code: :unsupported_docker_image_size_format, data: {value: size_field} unless match and match[1] and match[2]

                number = match[1].to_f
                unit = match[2].downcase

                coef = case unit
                       when 'b'  then 0
                       when 'kb' then 1
                       when 'mb' then 2
                       when 'gb' then 3
                       when 'tb' then 4
                       end

                number * (1000**coef)
              end
              cache[name] = { id: id, created_at: created_at, size: size }
            end
            #p [:cache_reset, name, :took, Time.now - t]
          end
        end
      end # Docker
    end # Image
  end # Dimg
end # Dapp
