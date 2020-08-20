package storage

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/golang/example/stringutil"

	"github.com/werf/werf/pkg/image"

	"github.com/werf/werf/pkg/container_runtime"

	"github.com/werf/logboek"
	"github.com/werf/werf/pkg/docker_registry"
)

const (
	RepoStage_ImageFormat = "%s:%s-%d"

	RepoManagedImageRecord_ImageTagPrefix  = "managed-image-"
	RepoManagedImageRecord_ImageNameFormat = "%s:managed-image-%s"

	RepoImageMetadataByCommitRecord_ImageTagPrefix  = "image-metadata-by-commit-"
	RepoImageMetadataByCommitRecord_ImageNameFormat = "%s:image-metadata-by-commit-%s-%s"

	RepoClientIDRecrod_ImageTagPrefix  = "client-id-"
	RepoClientIDRecrod_ImageNameFormat = "%s:client-id-%s-%d"

	UnexpectedTagFormatErrorPrefix = "unexpected tag format"
)

func getSignatureAndUniqueIDFromRepoStageImageTag(repoStageImageTag string) (string, int64, error) {
	parts := strings.SplitN(repoStageImageTag, "-", 2)

	if len(parts) != 2 {
		return "", 0, fmt.Errorf("%s %s", UnexpectedTagFormatErrorPrefix, repoStageImageTag)
	}

	if uniqueID, err := image.ParseUniqueIDAsTimestamp(parts[1]); err != nil {
		return "", 0, fmt.Errorf("%s %s: unable to parse unique id %s as timestamp: %s", UnexpectedTagFormatErrorPrefix, repoStageImageTag, parts[1], err)
	} else {
		return parts[0], uniqueID, nil
	}
}

func isUnexpectedTagFormatError(err error) bool {
	return strings.HasPrefix(err.Error(), UnexpectedTagFormatErrorPrefix)
}

type RepoStagesStorage struct {
	RepoAddress      string
	DockerRegistry   docker_registry.DockerRegistry
	ContainerRuntime container_runtime.ContainerRuntime
}

type RepoStagesStorageOptions struct {
	docker_registry.DockerRegistryOptions
	Implementation string
}

func NewRepoStagesStorage(repoAddress string, containerRuntime container_runtime.ContainerRuntime, options RepoStagesStorageOptions) (*RepoStagesStorage, error) {
	implementation := options.Implementation

	dockerRegistry, err := docker_registry.NewDockerRegistry(repoAddress, implementation, options.DockerRegistryOptions)
	if err != nil {
		return nil, fmt.Errorf("error creating docker registry accessor for repo %q: %s", repoAddress, err)
	}

	return &RepoStagesStorage{
		RepoAddress:      repoAddress,
		DockerRegistry:   dockerRegistry,
		ContainerRuntime: containerRuntime,
	}, nil
}

func (storage *RepoStagesStorage) ConstructStageImageName(projectName, signature string, uniqueID int64) string {
	return fmt.Sprintf(RepoStage_ImageFormat, storage.RepoAddress, signature, uniqueID)
}

func (storage *RepoStagesStorage) GetAllStages(projectName string) ([]image.StageID, error) {
	var res []image.StageID

	if tags, err := storage.DockerRegistry.Tags(storage.RepoAddress); err != nil {
		return nil, fmt.Errorf("unable to fetch tags for repo %q: %s", storage.RepoAddress, err)
	} else {
		logboek.Debug.LogF("-- RepoStagesStorage.GetRepoImagesBySignature fetched tags for %q: %#v\n", storage.RepoAddress, tags)

		for _, tag := range tags {
			if strings.HasPrefix(tag, RepoManagedImageRecord_ImageTagPrefix) || strings.HasPrefix(tag, RepoImageMetadataByCommitRecord_ImageTagPrefix) {
				continue
			}

			if signature, uniqueID, err := getSignatureAndUniqueIDFromRepoStageImageTag(tag); err != nil {
				if isUnexpectedTagFormatError(err) {
					logboek.Debug.LogLn(err.Error())
					continue
				}
				return nil, err
			} else {
				res = append(res, image.StageID{Signature: signature, UniqueID: uniqueID})

				logboek.Debug.LogF("Selected stage by signature %q uniqueID %d\n", signature, uniqueID)
			}
		}

		return res, nil
	}
}

func (storage *RepoStagesStorage) DeleteStages(options DeleteImageOptions, stages ...*image.StageDescription) error {
	var imageInfoList []*image.Info
	for _, stageDesc := range stages {
		imageInfoList = append(imageInfoList, stageDesc.Info)
	}
	return storage.DockerRegistry.DeleteRepoImage(imageInfoList...)
}

func (storage *RepoStagesStorage) CreateRepo() error {
	return storage.DockerRegistry.CreateRepo(storage.RepoAddress)
}

func (storage *RepoStagesStorage) DeleteRepo() error {
	return storage.DockerRegistry.DeleteRepo(storage.RepoAddress)
}

func (storage *RepoStagesStorage) GetStagesBySignature(projectName, signature string) ([]image.StageID, error) {
	var res []image.StageID

	if tags, err := storage.DockerRegistry.Tags(storage.RepoAddress); err != nil {
		return nil, fmt.Errorf("unable to fetch tags for repo %q: %s", storage.RepoAddress, err)
	} else {
		logboek.Debug.LogF("-- RepoStagesStorage.GetRepoImagesBySignature fetched tags for %q: %#v\n", storage.RepoAddress, tags)
		for _, tag := range tags {
			if !strings.HasPrefix(tag, signature) {
				logboek.Debug.LogF("Discard tag %q: should have prefix %q\n", tag, signature)
				continue
			}
			if _, uniqueID, err := getSignatureAndUniqueIDFromRepoStageImageTag(tag); err != nil {
				if isUnexpectedTagFormatError(err) {
					logboek.Debug.LogLn(err.Error())
					continue
				}
				return nil, err
			} else {
				logboek.Debug.LogF("Tag %q is suitable for signature %q\n", tag, signature)
				res = append(res, image.StageID{Signature: signature, UniqueID: uniqueID})
			}
		}
	}

	logboek.Debug.LogF("-- RepoStagesStorage.GetRepoImagesBySignature result for %q: %#v\n", storage.RepoAddress, res)

	return res, nil
}

func (storage *RepoStagesStorage) GetStageDescription(projectName, signature string, uniqueID int64) (*image.StageDescription, error) {
	stageImageName := storage.ConstructStageImageName(projectName, signature, uniqueID)

	logboek.Debug.LogF("-- RepoStagesStorage GetStageDescription %s %s %d\n", projectName, signature, uniqueID)
	logboek.Debug.LogF("-- RepoStagesStorage stageImageName = %q\n", stageImageName)

	if imgInfo, err := storage.DockerRegistry.TryGetRepoImage(stageImageName); err != nil {
		return nil, err
	} else if imgInfo != nil {
		return &image.StageDescription{
			StageID: &image.StageID{Signature: signature, UniqueID: uniqueID},
			Info:    imgInfo,
		}, nil
	}
	return nil, nil
}

func (storage *RepoStagesStorage) AddManagedImage(projectName, imageName string) error {
	logboek.Debug.LogF("-- RepoStagesStorage.AddManagedImage %s %s\n", projectName, imageName)

	if validateImageName(imageName) != nil {
		return nil
	}

	fullImageName := makeRepoManagedImageRecord(storage.RepoAddress, imageName)
	logboek.Debug.LogF("-- RepoStagesStorage.AddManagedImage full image name: %s\n", fullImageName)

	if isExists, err := storage.DockerRegistry.IsRepoImageExists(fullImageName); err != nil {
		return err
	} else if isExists {
		logboek.Debug.LogF("-- RepoStagesStorage.AddManagedImage record %q is exists => exiting\n", fullImageName)
		return nil
	}

	logboek.Debug.LogF("-- RepoStagesStorage.AddManagedImage record %q does not exist => creating record\n", fullImageName)

	if err := storage.DockerRegistry.PushImage(fullImageName, docker_registry.PushImageOptions{}); err != nil {
		return fmt.Errorf("unable to push image %s: %s", fullImageName, err)
	}

	return nil
}

func (storage *RepoStagesStorage) RmManagedImage(projectName, imageName string) error {
	logboek.Debug.LogF("-- RepoStagesStorage.RmManagedImage %s %s\n", projectName, imageName)

	fullImageName := makeRepoManagedImageRecord(storage.RepoAddress, imageName)

	if imgInfo, err := storage.DockerRegistry.TryGetRepoImage(fullImageName); err != nil {
		return fmt.Errorf("unable to get repo image %q info: %s", fullImageName, err)
	} else if imgInfo == nil {
		logboek.Debug.LogF("-- RepoStagesStorage.RmManagedImage record %q does not exist => exiting\n", fullImageName)
		return nil
	} else {
		if err := storage.DockerRegistry.DeleteRepoImage(imgInfo); err != nil {
			return fmt.Errorf("unable to delete image %q from repo: %s", fullImageName, err)
		}
	}

	return nil
}

func (storage *RepoStagesStorage) GetManagedImages(projectName string) ([]string, error) {
	logboek.Debug.LogF("-- RepoStagesStorage.GetManagedImages %s\n", projectName)

	var res []string

	if tags, err := storage.DockerRegistry.Tags(storage.RepoAddress); err != nil {
		return nil, fmt.Errorf("unable to get repo %s tags: %s", storage.RepoAddress, err)
	} else {
		for _, tag := range tags {
			if !strings.HasPrefix(tag, RepoManagedImageRecord_ImageTagPrefix) {
				continue
			}

			managedImageName := unslugDockerImageTagAsImageName(strings.TrimPrefix(tag, RepoManagedImageRecord_ImageTagPrefix))

			if validateImageName(managedImageName) != nil {
				continue
			}

			res = append(res, managedImageName)
		}
	}

	return res, nil
}

func (storage *RepoStagesStorage) FetchImage(img container_runtime.Image) error {
	switch containerRuntime := storage.ContainerRuntime.(type) {
	case *container_runtime.LocalDockerServerRuntime:
		return containerRuntime.PullImageFromRegistry(img)
	default:
		// TODO: case *container_runtime.LocalHostRuntime:
		panic("not implemented")
	}
}

func (storage *RepoStagesStorage) StoreImage(img container_runtime.Image) error {
	switch containerRuntime := storage.ContainerRuntime.(type) {
	case *container_runtime.LocalDockerServerRuntime:
		dockerImage := img.(*container_runtime.DockerImage)

		if dockerImage.Image.GetBuiltId() != "" {
			return containerRuntime.PushBuiltImage(img)
		} else {
			return containerRuntime.PushImage(img)
		}

	default:
		// TODO: case *container_runtime.LocalHostRuntime:
		panic("not implemented")
	}
}

func (storage *RepoStagesStorage) ShouldFetchImage(img container_runtime.Image) (bool, error) {
	switch storage.ContainerRuntime.(type) {
	case *container_runtime.LocalDockerServerRuntime:
		dockerImage := img.(*container_runtime.DockerImage)
		return !dockerImage.Image.IsExistsLocally(), nil
	default:
		panic("not implemented")
	}
}

func (storage *RepoStagesStorage) PutImageCommit(projectName, imageName, commit string, metadata *ImageMetadata) error {
	logboek.Debug.LogF("-- RepoStagesStorage.PutImageCommit %s %s %s %#v\n", projectName, imageName, commit, metadata)

	fullImageName := makeRepoImageMetadataByCommitImageRecord(storage.RepoAddress, imageName, commit)
	logboek.Debug.LogF("-- RepoStagesStorage.PutImageCommit full image name: %s\n", fullImageName)

	opts := docker_registry.PushImageOptions{
		Labels: map[string]string{"ContentSignature": metadata.ContentSignature},
	}
	if err := storage.DockerRegistry.PushImage(fullImageName, opts); err != nil {
		return fmt.Errorf("unable to push image %s with metadata: %s", fullImageName, err)
	}

	logboek.Info.LogF("Put content-signature %q into metadata for image %q by commit %s\n", metadata.ContentSignature, imageName, commit)

	return nil
}

func (storage *RepoStagesStorage) RmImageCommit(projectName, imageName, commit string) error {
	logboek.Debug.LogF("-- RepoStagesStorage.RmImageCommit %s %s %s\n", projectName, imageName, commit)

	fullImageName := makeRepoImageMetadataByCommitImageRecord(storage.RepoAddress, imageName, commit)
	logboek.Debug.LogF("-- RepoStagesStorage.RmImageCommit full image name: %s\n", fullImageName)

	if img, err := storage.DockerRegistry.TryGetRepoImage(fullImageName); err != nil {
		return fmt.Errorf("unable to get repo image %s: %s", fullImageName, err)
	} else if img != nil {
		if err := storage.DockerRegistry.DeleteRepoImage(img); err != nil {
			return fmt.Errorf("unable to remove repo image %s: %s", fullImageName, err)
		}

		logboek.Info.LogF("Removed image %q metadata by commit %s\n", imageName, commit)
	}

	return nil
}

func (storage *RepoStagesStorage) GetImageMetadataByCommit(projectName, imageName, commit string) (*ImageMetadata, error) {
	logboek.Debug.LogF("-- RepoStagesStorage.GetImageStagesSignatureByCommit %s %s %s\n", projectName, imageName, commit)

	fullImageName := makeRepoImageMetadataByCommitImageRecord(storage.RepoAddress, imageName, commit)
	logboek.Debug.LogF("-- RepoStagesStorage.GetImageStagesSignatureByCommit full image name: %s\n", fullImageName)

	if imgInfo, err := storage.DockerRegistry.TryGetRepoImage(fullImageName); err != nil {
		return nil, fmt.Errorf("unable to get repo image %s: %s", fullImageName, err)
	} else if imgInfo != nil && imgInfo.Labels != nil {
		metadata := &ImageMetadata{ContentSignature: imgInfo.Labels["ContentSignature"]}

		logboek.Debug.LogF("Got content-signature %q from image %q metadata by commit %s\n", metadata.ContentSignature, imageName, commit)

		return metadata, nil
	} else {
		logboek.Debug.LogF("imgInfo = %v\n", imgInfo)
		if imgInfo != nil {
			logboek.Debug.LogF("imgInfo.Labels = %v\n", imgInfo.Labels)
		}

		logboek.Info.LogF("No metadata found for image %q by commit %s\n", imageName, commit)
		return nil, nil
	}
}

func (storage *RepoStagesStorage) GetImageCommits(projectName, imageName string) ([]string, error) {
	logboek.Debug.LogF("-- RepoStagesStorage.GetImageCommits %s %s\n", projectName, imageName)

	var res []string

	if tags, err := storage.DockerRegistry.Tags(storage.RepoAddress); err != nil {
		return nil, fmt.Errorf("unable to get repo %s tags: %s", storage.RepoAddress, err)
	} else {
		for _, tag := range tags {
			if !strings.HasPrefix(tag, RepoImageMetadataByCommitRecord_ImageTagPrefix) {
				continue
			}

			sluggedImageAndCommit := strings.TrimPrefix(tag, RepoImageMetadataByCommitRecord_ImageTagPrefix)

			sluggedImageAndCommitParts := strings.Split(sluggedImageAndCommit, "-")
			if len(sluggedImageAndCommitParts) < 2 {
				// unexpected
				continue
			}

			commit := sluggedImageAndCommitParts[len(sluggedImageAndCommitParts)-1]
			sluggedImage := strings.TrimSuffix(sluggedImageAndCommit, fmt.Sprintf("-%s", commit))
			iName := unslugDockerImageTagAsImageName(sluggedImage)

			if imageName == iName {
				logboek.Debug.LogF("Found image %q metadata by commit %s, full image name: %s:%s\n", imageName, commit, storage.RepoAddress, tag)
				res = append(res, commit)
			}
		}
	}

	return res, nil
}

func makeRepoImageMetadataByCommitImageRecord(repoAddress, imageName, commit string) string {
	return fmt.Sprintf(RepoImageMetadataByCommitRecord_ImageNameFormat, repoAddress, slugImageNameAsDockerImageTag(imageName), commit)
}

func (storage *RepoStagesStorage) String() string {
	return fmt.Sprintf("repo stages storage (%q)", storage.RepoAddress)
}

func (storage *RepoStagesStorage) Address() string {
	return storage.RepoAddress
}

func makeRepoManagedImageRecord(repoAddress, imageName string) string {
	return fmt.Sprintf(RepoManagedImageRecord_ImageNameFormat, repoAddress, slugImageNameAsDockerImageTag(imageName))
}

func slugImageNameAsDockerImageTag(imageName string) string {
	res := imageName
	res = strings.ReplaceAll(res, "/", "__slash__")
	res = strings.ReplaceAll(res, "+", "__plus__")

	if imageName == "" {
		res = NamelessImageRecordTag
	}

	return res
}

func unslugDockerImageTagAsImageName(tag string) string {
	res := tag
	res = strings.ReplaceAll(res, "__slash__", "/")
	res = strings.ReplaceAll(res, "__plus__", "+")

	if res == NamelessImageRecordTag {
		res = ""
	}

	return res
}

func validateImageName(name string) error {
	if strings.ToLower(name) != name {
		return fmt.Errorf("no upcase symbols allowed")
	}
	return nil
}

func (storage *RepoStagesStorage) GetClientIDRecords(projectName string) ([]*ClientIDRecord, error) {
	logboek.Debug.LogF("-- RepoStagesStorage.GetClientIDRecords for project %s\n", projectName)

	var res []*ClientIDRecord

	if tags, err := storage.DockerRegistry.Tags(storage.RepoAddress); err != nil {
		return nil, fmt.Errorf("unable to get repo %s tags: %s", storage.RepoAddress, err)
	} else {
		for _, tag := range tags {
			if !strings.HasPrefix(tag, RepoClientIDRecrod_ImageTagPrefix) {
				continue
			}

			tagWithoutPrefix := strings.TrimPrefix(tag, RepoClientIDRecrod_ImageTagPrefix)
			dataParts := strings.SplitN(stringutil.Reverse(tagWithoutPrefix), "-", 2)
			if len(dataParts) != 2 {
				continue
			}

			clientID, timestampMillisecStr := stringutil.Reverse(dataParts[1]), stringutil.Reverse(dataParts[0])

			timestampMillisec, err := strconv.ParseInt(timestampMillisecStr, 10, 64)
			if err != nil {
				continue
			}

			rec := &ClientIDRecord{ClientID: clientID, TimestampMillisec: timestampMillisec}
			res = append(res, rec)

			logboek.Debug.LogF("-- RepoStagesStorage.GetClientIDRecords got clientID record: %s\n", rec)
		}
	}

	return res, nil
}

func (storage *RepoStagesStorage) PostClientIDRecord(projectName string, rec *ClientIDRecord) error {
	logboek.Debug.LogF("-- RepoStagesStorage.PostClientID %s for project %s\n", rec.ClientID, projectName)

	fullImageName := fmt.Sprintf(RepoClientIDRecrod_ImageNameFormat, storage.RepoAddress, rec.ClientID, rec.TimestampMillisec)

	logboek.Debug.LogF("-- RepoStagesStorage.PostClientID full image name: %s\n", fullImageName)

	if isExists, err := storage.DockerRegistry.IsRepoImageExists(fullImageName); err != nil {
		return err
	} else if isExists {
		logboek.Debug.LogF("-- RepoStagesStorage.AddManagedImage record %q is exists => exiting\n", fullImageName)
		return nil
	}

	if err := storage.DockerRegistry.PushImage(fullImageName, docker_registry.PushImageOptions{}); err != nil {
		return fmt.Errorf("unable to push image %s: %s", fullImageName, err)
	}

	logboek.Info.LogF("Posted new clientID %q for project %s\n", rec.ClientID, projectName)

	return nil
}
