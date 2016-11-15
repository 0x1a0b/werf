require_relative 'spec_helper'

describe Dapp::GitArtifact do
  before :all do
    shellout 'git config -l | grep "user.email" || git config --global user.email "dapp@flant.com"'
    shellout 'git config -l | grep "user.name" || git config --global user.name "Dapp Dapp"'
  end

  before :each do
    @builder = instance_double('Dapp::Builder')
    allow(@builder).to receive(:register_atomizer)
    allow(@builder).to receive(:build_path) do |*args|
      File.absolute_path(File.join(*args))
    end
    allow(@builder).to receive(:home_path).and_return('')
    allow(@builder).to receive(:shellout) do |*args, **kwargs|
      shellout(*args, **kwargs)
    end
    allow(@builder).to receive(:filelock).and_yield

    @docker = instance_double('Dapp::Docker')
    allow(@docker).to receive(:add_artifact)
    allow(@docker).to receive(:run)
    allow(@builder).to receive(:docker).and_return(@docker)

    @repo = Dapp::GitRepo::Chronicler.new(@builder, 'repo')
  end

  def reset_instances
    RSpec::Mocks.space.proxy_for(@builder).instance_variable_get(:@messages_received).clear
    RSpec::Mocks.space.proxy_for(@docker).instance_variable_get(:@messages_received).clear
  end

  def repo_create_branch(branch)
    shellout "git branch #{branch}", cwd: @repo.name
  end

  def repo_change_and_commit(changefile: 'data.txt', changedata: random_string, branch: 'master')
    shellout "git checkout #{branch}", cwd: @repo.name
    changefile = File.join(@repo.name, changefile)
    FileUtils.mkdir_p File.split(changefile)[0]
    File.write changefile, changedata
    @repo.commit!
  end

  def artifact(id: nil)
    (@artifacts ||= {})[id]
  end

  # rubocop:disable Metrics/ParameterLists
  def artifact_init(where_to_add, id: nil, changefile: 'data.txt', changedata: random_string, branch: 'master', **kwargs)
    repo_change_and_commit(changefile: changefile, changedata: changedata, branch: branch)

    (@artifacts ||= {})[id] = Dapp::GitArtifact.new(@builder, @repo, where_to_add, branch: branch, **kwargs)
  end
  # rubocop:enable Metrics/ParameterLists

  def artifact_reset(id: nil)
    @artifacts.delete(id).send(:atomizer).tap do |atomizer|
      atomizer.commit!
      atomizer.instance_variable_get(:@file).close
    end
  end

  def artifact_filename(ending, id: nil)
    "#{artifact(id: id).repo.name}#{artifact(id: id).name ? "_#{artifact(id: id).name}" : nil}.#{artifact(id: id).branch}#{ending}"
  end

  def tar_files_owners(arhive)
    shellout("tar -tvf #{arhive}").stdout.lines.map { |s| s.strip.sub(%r(.{11}([^\/]+)\/.*), '\1') }.uniq
  end

  def tar_files_groups(arhive)
    shellout("tar -tvf #{arhive}").stdout.lines.map { |s| s.strip.sub(%r(.{11}[^\/]+\/([^\s]+).*), '\1') }.uniq
  end

  def artifact_tar_files(id: nil)
    shellout("tar -tf #{artifact_filename('.tar.gz', id: id)}").stdout.lines.map(&:strip).select { |f| !(f =~ %r{/$}) }
  end

  # rubocop:disable Metrics/AbcSize
  def artifact_archive(id: nil)
    reset_instances
    artifact(id: id).add_multilayer!

    expect(@docker).to have_received(:add_artifact).with(
      %r{\/#{artifact_filename('.tar.gz', id: id)}$},
      artifact_filename('.tar.gz', id: id),
      artifact(id: id).where_to_add,
      step: :prepare
    )
    expect(File.read(artifact_filename('.commit', id: id)).strip).to eq(@repo.latest_commit(artifact(id: id).branch))
    expect(File.exist?(artifact_filename('.tar.gz', id: id))).to be_truthy

    [:owner, :group].each do |subj|
      if artifact(id: id).send(subj)
        expect(send(:"tar_files_#{subj}s", artifact_filename('.tar.gz', id: id))).to eq([artifact(id: id).send(subj).to_s])
      end
    end
  end
  # rubocop:enable Metrics/AbcSize

  def random_string
    (('a'..'z').to_a * 10).sample(100).join
  end

  def artifact_latest_patch(id: nil, **kwargs)
    artifact_patch(
      '_latest',
      :setup,
      id: id,
      **kwargs
    )
  end

  def artifact_layer_patch(layer, id: nil, **kwargs)
    Timecop.travel(Time.now + artifact(id: id).interlayer_period)

    artifact_patch(
      format('_layer_%04d', layer),
      :build,
      id: id,
      **kwargs
    )
  ensure
    Timecop.return
  end

  # rubocop:disable Metrics/AbcSize, Metrics/ParameterLists, Metrics/MethodLength
  def artifact_patch(suffix, step, id:, changefile: 'data.txt', changedata: random_string, should_be_empty: false, **_kwargs)
    repo_change_and_commit(changefile: changefile, changedata: changedata, branch: artifact(id: id).branch)

    reset_instances
    artifact(id: id).add_multilayer!

    patch_filename = artifact_filename("#{suffix}.patch.gz", id: id)
    patch_filename_esc = Regexp.escape(patch_filename)
    commit_filename = artifact_filename("#{suffix}.commit", id: id)

    if should_be_empty
      expect(@docker).to_not have_received(:add_artifact).with(/#{patch_filename_esc}$/, patch_filename, '/tmp', step: step)
      expect(@docker).to_not have_received(:run).with(/#{patch_filename_esc}/, /#{patch_filename_esc}$/, step: step)
      expect(File.exist?(patch_filename)).to be_falsy
      expect(File.exist?(commit_filename)).to be_falsy
    else
      expect(@docker).to have_received(:add_artifact).with(/#{patch_filename_esc}$/, patch_filename, '/tmp', step: step)
      expect(@docker).to have_received(:run).with(
        %r{^zcat \/tmp\/#{patch_filename_esc} \| .*git apply --whitespace=nowarn --directory=#{artifact(id: id).where_to_add}$},
        "rm /tmp/#{patch_filename}",
        step: step
      )
      { owner: 'u', group: 'g' }.each do |subj, flag|
        if (value = artifact(id: id).send(subj))
          value = "\\\\##{value}" if value.to_i.to_s == value.to_s
          expect(@docker).to have_received(:run).with(/#{patch_filename_esc} \| sudo.*-#{flag} #{value}.*git apply/, any_args)
        end
      end
      expect(File.read(commit_filename).strip).to eq(@repo.latest_commit(artifact(id: id).branch))
      expect(File.exist?(patch_filename)).to be_truthy
      expect(File.exist?(commit_filename)).to be_truthy
      expect(shellout("zcat #{patch_filename}").stdout).to match(/#{changedata}/)
    end
  end
  # rubocop:enable Metrics/AbcSize, Metrics/ParameterLists, Metrics/MethodLength

  def artifact_do_test(where_to_add, latest_patch: true, layers: 3, **kwargs)
    artifact_init where_to_add, **kwargs
    artifact_archive
    layers.times do |i|
      artifact_layer_patch i + 1
    end
    artifact_latest_patch if latest_patch
  end

  def artifact_expect_clean(id: nil)
    expect(Dir.glob(artifact_filename('{.,_}*', id: id)))
      .to match_array([artifact_filename('.paramshash', id: id), artifact_filename('.atomizer', id: id)])
  end

  it '#archive_only', test_construct: true do
    artifact_do_test '/dest', latest_patch: false, layers: 0
  end

  it '#latest_patch', test_construct: true do
    artifact_do_test '/dest', layers: 0
  end

  it '#layer_patch', test_construct: true do
    artifact_do_test '/dest', latest_patch: false, layers: 1
  end

  it '#layer_patch_and_latest_patch', test_construct: true do
    artifact_do_test '/dest', layers: 1
  end

  it '#multiple_layer_patches_and_latest_patch', test_construct: true do
    artifact_do_test '/dest'
  end

  it '#multiple_artifacts', test_construct: true do
    artifact_init '/dest', name: 'a', id: :a
    artifact_init '/dest_2', name: 'b', id: :b
    artifact_archive id: :b
    artifact_archive id: :a

    artifact_layer_patch 1, id: :a
    artifact_layer_patch 1, id: :b
    artifact_layer_patch 2, id: :b
    artifact_layer_patch 2, id: :a

    artifact_latest_patch id: :b
    artifact_latest_patch id: :a
    artifact_latest_patch id: :a
    artifact_latest_patch id: :b

    artifact_reset id: :a
    artifact_reset id: :b

    artifact_init '/dest', name: 'a', id: :a
    artifact_init '/dest_2', name: 'b', id: :b
    artifact_latest_patch id: :a
    artifact_latest_patch id: :b
  end

  it '#remove_latest_patch_if_no_more_diff', test_construct: true do
    artifact_init '/dest', changedata: 'text'
    artifact_archive
    artifact_latest_patch
    artifact_latest_patch changedata: 'text', should_be_empty: true

    3.times do |i|
      artifact_layer_patch i + 1, changedata: "text_#{i}"
      artifact_latest_patch
      artifact_latest_patch changedata: "text_#{i}", should_be_empty: true
    end
  end

  { cwd: 'x', paths: 'x', owner: 70_500, group: 70_500 }.each do |param, value|
    it "#autocleanup_on_#{param}_change", test_construct: true do
      artifact_do_test '/dest', layers: 2

      artifact_reset
      artifact_init '/dest', **{ param => value }
      artifact_expect_clean
    end
  end

  class << self
    def users_and_groups_to_test
      users = [nil, 'root', 100_500]
      users << 'some_unknown' unless shellout('lsb_release -cs').stdout.strip == 'precise'
      users.product(users)
    end
  end

  users_and_groups_to_test.each do |owner, group|
    it "#change_owner_to_#{owner}_and_group_to_#{group}", test_construct: true do
      artifact_do_test '/dest', owner: owner, group: group
    end
  end

  it '#interlayer_period', test_construct: true do
    artifact_do_test '/dest', interlayer_period: 10
  end

  it '#flush_cache', test_construct: true do
    artifact_do_test '/dest'
    artifact_reset
    artifact_init '/dest', flush_cache: true
    artifact_expect_clean
  end

  it '#branch', test_construct: true do
    repo_create_branch 'not_master'

    artifact_init '/dest', branch: 'not_master', changedata: 'text'
    artifact_archive
    repo_change_and_commit branch: 'master'
    artifact_latest_patch changedata: 'text', should_be_empty: true

    3.times do |i|
      artifact_layer_patch i + 1, changedata: "text_#{i}"
      repo_change_and_commit branch: 'master'
      artifact_latest_patch changedata: "text_#{i}", should_be_empty: true
    end
  end

  it '#commit_by_step', test_construct: true do
    artifact_init '/dest'

    artifact_archive
    expect(artifact.commit_by_step(:prepare)).to eq(artifact.commit_by_step(:build))
    expect(artifact.commit_by_step(:build)).to eq(artifact.commit_by_step(:setup))

    artifact_latest_patch changedata: 'text'
    expect(artifact.commit_by_step(:build)).to eq(artifact.commit_by_step(:prepare))
    expect(artifact.commit_by_step(:setup)).not_to eq(artifact.commit_by_step(:build))

    artifact_layer_patch 1
    expect(artifact.commit_by_step(:build)).not_to eq(artifact.commit_by_step(:prepare))
    expect(artifact.commit_by_step(:setup)).to eq(artifact.commit_by_step(:build))

    artifact_latest_patch changedata: 'text'
    expect(artifact.commit_by_step(:build)).not_to eq(artifact.commit_by_step(:prepare))
    expect(artifact.commit_by_step(:setup)).not_to eq(artifact.commit_by_step(:build))
    expect(artifact.commit_by_step(:prepare)).not_to eq(artifact.commit_by_step(:setup))
  end

  it '#exist_in_step', test_construct: true do
    artifact_init '/dest', changefile: 'data1.txt'
    artifact_archive
    expect(artifact.exist_in_step?('data1.txt', :prepare)).to be_truthy

    artifact_latest_patch changefile: 'data2.txt'
    expect(artifact.exist_in_step?('data1.txt', :setup)).to be_truthy
    expect(artifact.exist_in_step?('data2.txt', :setup)).to be_truthy
    expect(artifact.exist_in_step?('data2.txt', :build)).to be_falsy
    expect(artifact.exist_in_step?('data2.txt', :prepare)).to be_falsy

    artifact_layer_patch 1, changefile: 'data3.txt'
    expect(artifact.exist_in_step?('data1.txt', :build)).to be_truthy
    expect(artifact.exist_in_step?('data2.txt', :build)).to be_truthy
    expect(artifact.exist_in_step?('data3.txt', :build)).to be_truthy
    expect(artifact.exist_in_step?('data3.txt', :prepare)).to be_falsy

    artifact_latest_patch changefile: 'data4.txt'
    expect(artifact.exist_in_step?('data1.txt', :setup)).to be_truthy
    expect(artifact.exist_in_step?('data2.txt', :setup)).to be_truthy
    expect(artifact.exist_in_step?('data3.txt', :setup)).to be_truthy
    expect(artifact.exist_in_step?('data4.txt', :setup)).to be_truthy
    expect(artifact.exist_in_step?('data4.txt', :build)).to be_falsy
    expect(artifact.exist_in_step?('data4.txt', :prepare)).to be_falsy
  end

  it '#cwd', test_construct: true do
    artifact_init '/dest', cwd: 'a', changefile: 'a/data.txt'
    artifact_archive
    expect(artifact_tar_files).to match_array('data.txt')

    artifact_latest_patch should_be_empty: true
    artifact_layer_patch 1, should_be_empty: true

    artifact_latest_patch changefile: 'a/data.txt'
    artifact_layer_patch 1, changefile: 'a/data.txt'

    artifact_latest_patch should_be_empty: true
    artifact_latest_patch changefile: 'a/data.txt'
  end

  it '#paths', test_construct: true do
    repo_change_and_commit changefile: 'x/data.txt'
    repo_change_and_commit changefile: 'x/y/data.txt'
    repo_change_and_commit changefile: 'z/data.txt'
    artifact_init '/dest', paths: ['x/y', 'z']

    artifact_archive
    expect(artifact_tar_files).to match_array(['x/y/data.txt', 'z/data.txt'])

    artifact_latest_patch should_be_empty: true
    repo_change_and_commit changefile: 'x/data.txt'
    artifact_latest_patch should_be_empty: true
    artifact_layer_patch 1, should_be_empty: true
    repo_change_and_commit changefile: 'x/data.txt'

    artifact_latest_patch changefile: 'x/y/data.txt'
    artifact_layer_patch 1, changefile: 'z/data.txt'

    artifact_latest_patch should_be_empty: true
    repo_change_and_commit changefile: 'x/data.txt'
    artifact_latest_patch should_be_empty: true
    artifact_latest_patch changefile: 'x/y/data.txt'
  end

  it '#cwd_and_paths', test_construct: true do
    repo_change_and_commit changefile: 'a/data.txt'
    repo_change_and_commit changefile: 'a/x/data.txt'
    repo_change_and_commit changefile: 'a/x/y/data.txt'
    repo_change_and_commit changefile: 'a/z/data.txt'
    artifact_init '/dest', cwd: 'a', paths: ['x/y', 'z']

    artifact_archive
    expect(artifact_tar_files).to match_array(['x/y/data.txt', 'z/data.txt'])

    artifact_latest_patch should_be_empty: true
    repo_change_and_commit changefile: 'a/data.txt'
    artifact_latest_patch should_be_empty: true
    repo_change_and_commit changefile: 'a/x/data.txt'
    artifact_latest_patch should_be_empty: true
    artifact_layer_patch 1, should_be_empty: true
    repo_change_and_commit changefile: 'a/data.txt'
    artifact_layer_patch 1, should_be_empty: true
    repo_change_and_commit changefile: 'a/x/data.txt'

    artifact_latest_patch changefile: 'a/x/y/data.txt'
    artifact_layer_patch 1, changefile: 'a/z/data.txt'

    artifact_latest_patch should_be_empty: true
    repo_change_and_commit changefile: 'a/data.txt'
    artifact_latest_patch should_be_empty: true
    repo_change_and_commit changefile: 'a/x/data.txt'
    artifact_latest_patch should_be_empty: true
    artifact_latest_patch changefile: 'a/x/y/data.txt'
  end

  it '#file_removal_in_patch', test_construct: true do
    artifact_init '/dest', changedata: 'test'
    repo_change_and_commit changefile: 'data2.txt', changedata: 'test'

    artifact_archive
    FileUtils.rm File.join(@repo.name, 'data2.txt')

    artifact_latest_patch changedata: 'test'
    expect(shellout("zcat #{artifact_filename('_latest.patch.gz')}").stdout).to match(%r{^\+\+\+ /dev/null$})
  end
end
