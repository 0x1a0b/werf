package cleanup

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"

	"github.com/flant/werf/pkg/build"
	"github.com/flant/werf/pkg/docker_registry"
	"github.com/flant/werf/pkg/image"
)

const syncIgnoreProjectImageStagePeriod = 2 * 60 * 60

func repoImageStagesSyncByRepoImages(repoImages []docker_registry.RepoImage, options CommonRepoOptions) error {
	repoImageStages, err := repoImageStagesImages(options)
	if err != nil {
		return err
	}

	if len(repoImageStages) == 0 {
		return nil
	}

	for _, repoImage := range repoImages {
		parentId, err := repoImageParentId(repoImage)
		if err != nil {
			return err
		}

		repoImageStages, err = exceptRepoImageStagesByImageId(repoImageStages, parentId)
		if err != nil {
			return err
		}
	}

	err = repoImagesRemove(repoImageStages, options)
	if err != nil {
		return err
	}

	return nil
}

func repoImageStagesSyncByCacheVersion(options CommonRepoOptions) error {
	repoImageStages, err := repoImageStagesImages(options)
	if err != nil {
		return err
	}

	var repoImagesToDelete []docker_registry.RepoImage
	for _, repoImageStage := range repoImageStages {
		labels, err := repoImageLabels(repoImageStage)
		if err != nil {
			return err
		}

		version, ok := labels[image.WerfCacheVersionLabel]
		if !ok || (version != build.BuildCacheVersion) {
			fmt.Printf("%s %s %s\n", repoImageStage.Tag, version, build.BuildCacheVersion)
			repoImagesToDelete = append(repoImagesToDelete, repoImageStage)
		}
	}

	if err := repoImagesRemove(repoImagesToDelete, options); err != nil {
		return err
	}

	return nil
}

func exceptRepoImageStagesByImageId(repoImageStages []docker_registry.RepoImage, imageId string) ([]docker_registry.RepoImage, error) {
	repoImageStage, err := findRepoImageStageByImageId(repoImageStages, imageId)
	if repoImageStage == nil {
		return repoImageStages, nil
	}

	repoImageStages, err = exceptRepoImageStagesByRepoImageStage(repoImageStages, *repoImageStage)
	if err != nil {
		return nil, err
	}

	return repoImageStages, nil
}

func findRepoImageStageByImageId(repoImageStages []docker_registry.RepoImage, imageId string) (*docker_registry.RepoImage, error) {
	for _, repoImageStage := range repoImageStages {
		manifest, err := repoImageStage.Manifest()
		if err != nil {
			return nil, err
		}

		repoImageStageImageId := manifest.Config.Digest.String()
		if repoImageStageImageId == imageId {
			return &repoImageStage, nil
		}
	}

	return nil, nil
}

func exceptRepoImageStagesByRepoImageStage(repoImageStages []docker_registry.RepoImage, repoImageStage docker_registry.RepoImage) ([]docker_registry.RepoImage, error) {
	labels, err := repoImageLabels(repoImageStage)
	if err != nil {
		return nil, err
	}

	for label, signature := range labels {
		if strings.HasPrefix(label, "werf-artifact") {
			repoImageStages, err = exceptRepoImageStagesBySignature(repoImageStages, signature)
			if err != nil {
				return nil, err
			}
		}
	}

	currentRepoImageStage := &repoImageStage
	for {
		repoImageStages = exceptRepoImages(repoImageStages, *currentRepoImageStage)

		parentId, err := repoImageParentId(*currentRepoImageStage)
		if err != nil {
			return nil, err
		}

		currentRepoImageStage, err = findRepoImageStageByImageId(repoImageStages, parentId)
		if err != nil {
			return nil, err
		}

		if currentRepoImageStage == nil {
			break
		}
	}

	return repoImageStages, nil
}

func exceptRepoImageStagesBySignature(repoImageStages []docker_registry.RepoImage, signature string) ([]docker_registry.RepoImage, error) {
	repoImageStage, err := findRepoImageStageBySignature(repoImageStages, signature)
	if repoImageStage == nil {
		return repoImageStages, nil
	}

	repoImageStages, err = exceptRepoImageStagesByRepoImageStage(repoImageStages, *repoImageStage)
	if err != nil {
		return nil, err
	}

	return repoImageStages, nil
}

func findRepoImageStageBySignature(repoImageStages []docker_registry.RepoImage, signature string) (*docker_registry.RepoImage, error) {
	for _, repoImageStage := range repoImageStages {
		if repoImageStage.Tag == fmt.Sprintf(build.RepoImageStageTagFormat, signature) {
			return &repoImageStage, nil
		}
	}

	return nil, nil
}

func repoImageParentId(repoImage docker_registry.RepoImage) (string, error) {
	configFile, err := repoImage.Image.ConfigFile()
	if err != nil {
		return "", err
	}

	return configFile.ContainerConfig.Image, nil
}

func repoImageLabels(repoImage docker_registry.RepoImage) (map[string]string, error) {
	configFile, err := repoImage.Image.ConfigFile()
	if err != nil {
		return nil, err
	}

	return configFile.Config.Labels, nil
}

func repoImageCreated(repoImage docker_registry.RepoImage) (time.Time, error) {
	configFile, err := repoImage.Image.ConfigFile()
	if err != nil {
		return time.Time{}, err
	}

	return configFile.Created.Time, nil
}

func projectImageStagesSyncByRepoImages(repoImages []docker_registry.RepoImage, options CommonProjectOptions) error {
	imageStages, err := projectImageStages(options)
	if err != nil {
		return err
	}

	for _, repoImage := range repoImages {
		parentId, err := repoImageParentId(repoImage)
		if err != nil {
			return err
		}

		imageStages, err = exceptImageStagesByImageId(imageStages, parentId, options)
		if err != nil {
			return err
		}
	}

	if os.Getenv("WERF_DISABLE_SYNC_LOCAL_STAGES_DATE_PERIOD_POLICY") == "" {
		for _, imageStage := range imageStages {
			if time.Now().Unix()-imageStage.Created < syncIgnoreProjectImageStagePeriod {
				imageStages = exceptImage(imageStages, imageStage)
			}
		}
	}

	err = imagesRemove(imageStages, options.CommonOptions)
	if err != nil {
		return err
	}

	return nil
}

func exceptImageStagesByImageId(imageStages []types.ImageSummary, imageId string, options CommonProjectOptions) ([]types.ImageSummary, error) {
	imageStage := findImageStageByImageId(imageStages, imageId)
	if imageStage == nil {
		return imageStages, nil
	}

	imageStages, err := exceptImageStagesByImageStage(imageStages, *imageStage, options)
	if err != nil {
		return nil, err
	}

	return imageStages, nil
}

func exceptImageStagesByImageStage(imageStages []types.ImageSummary, imageStage types.ImageSummary, commonProjectOptions CommonProjectOptions) ([]types.ImageSummary, error) {
	var err error
	for label, value := range imageStage.Labels {
		if strings.HasPrefix(label, "werf-artifact") {
			imageStages, err = exceptImageStagesBySignarute(imageStages, value, commonProjectOptions)
			if err != nil {
				return nil, err
			}
		}
	}

	currentImageStage := &imageStage
	for {
		imageStages = exceptImage(imageStages, *currentImageStage)
		currentImageStage = findImageStageByImageId(imageStages, currentImageStage.ParentID)
		if currentImageStage == nil {
			break
		}
	}

	return imageStages, nil
}

func exceptImageStagesBySignarute(imageStages []types.ImageSummary, signature string, options CommonProjectOptions) ([]types.ImageSummary, error) {
	imageStage := findImageStageBySignature(imageStages, signature, options)
	if imageStage == nil {
		return imageStages, nil
	}

	imageStages, err := exceptImageStagesByImageStage(imageStages, *imageStage, options)
	if err != nil {
		return nil, err
	}

	return imageStages, nil
}

func findImageStageBySignature(imageStages []types.ImageSummary, signature string, options CommonProjectOptions) *types.ImageSummary {
	targetImageStageName := stageCacheImage(signature, options)
	for _, imageStage := range imageStages {
		for _, imageStageName := range imageStage.RepoTags {
			if imageStageName == targetImageStageName {
				return &imageStage
			}

		}
	}

	return nil
}

func stageCacheImage(signature string, options CommonProjectOptions) string {
	return fmt.Sprintf(build.LocalImageStageImageFormat, options.ProjectName, signature)
}

func findImageStageByImageId(imageStages []types.ImageSummary, imageId string) *types.ImageSummary {
	for _, imageStage := range imageStages {
		if imageStage.ID == imageId {
			return &imageStage
		}
	}

	return nil
}

func projectImageStages(options CommonProjectOptions) ([]types.ImageSummary, error) {
	images, err := werfImagesByFilterSet(projectImageStageFilterSet(options))
	if err != nil {
		return nil, err
	}

	return images, nil
}

func projectImageStagesSyncByCacheVersion(options CommonProjectOptions) error {
	return werfImageStagesFlushByCacheVersion(projectImageStageFilterSet(options), options.CommonOptions)
}
