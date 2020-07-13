package build

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/werf/logboek"

	"github.com/werf/werf/pkg/build/stage"
	"github.com/werf/werf/pkg/container_runtime"
	"github.com/werf/werf/pkg/image"
	"github.com/werf/werf/pkg/storage"
	"github.com/werf/werf/pkg/tag_strategy"
	"github.com/werf/werf/pkg/util"
)

func NewPublishImagesPhase(c *Conveyor, imagesRepo storage.ImagesRepo, opts PublishImagesOptions) *PublishImagesPhase {
	tagsByScheme := map[tag_strategy.TagStrategy][]string{
		tag_strategy.Custom:    opts.CustomTags,
		tag_strategy.GitBranch: opts.TagsByGitBranch,
		tag_strategy.GitTag:    opts.TagsByGitTag,
		tag_strategy.GitCommit: opts.TagsByGitCommit,
	}
	return &PublishImagesPhase{
		BasePhase:            BasePhase{c},
		ImagesToPublish:      opts.ImagesToPublish,
		TagsByScheme:         tagsByScheme,
		TagByStagesSignature: opts.TagByStagesSignature,
		ImagesRepo:           imagesRepo,
		PublishReport:        &PublishReport{Images: make(map[string]PublishReportImageRecord)},
		PublishReportPath:    opts.PublishReportPath,
		PublishReportFormat:  opts.PublishReportFormat,
	}
}

type PublishImagesPhase struct {
	BasePhase
	ImagesToPublish      []string
	TagsByScheme         map[tag_strategy.TagStrategy][]string
	TagByStagesSignature bool
	ImagesRepo           storage.ImagesRepo

	PublishReport       *PublishReport
	PublishReportPath   string
	PublishReportFormat PublishReportFormat
}

type PublishReportFormat string

const (
	PublishReportJSON PublishReportFormat = "json"
)

type PublishReport struct {
	Images map[string]PublishReportImageRecord
}

type PublishReportImageRecord struct {
	WerfImageName string
	DockerRepo    string
	DockerTag     string
	DockerImageID string
}

func (phase *PublishImagesPhase) Name() string {
	return "publish"
}

func (phase *PublishImagesPhase) BeforeImages() error {
	return nil
}

func (phase *PublishImagesPhase) AfterImages() error {
	if data, err := json.Marshal(phase.PublishReport); err != nil {
		return fmt.Errorf("unable to prepare publish report: %s", err)
	} else {
		logboek.Debug.LogF("Publish report:\n%s\n", data)

		if phase.PublishReportPath != "" && phase.PublishReportFormat == PublishReportJSON {
			if err := ioutil.WriteFile(phase.PublishReportPath, append(data, []byte("\n")...), 0644); err != nil {
				return fmt.Errorf("unable to write publish report to %s: %s", phase.PublishReportPath, err)
			}
		}
	}

	return nil
}

func (phase *PublishImagesPhase) BeforeImageStages(img *Image) error {
	return nil
}

func (phase *PublishImagesPhase) OnImageStage(img *Image, stg stage.Interface) error {
	return nil
}

func (phase *PublishImagesPhase) AfterImageStages(img *Image) error {
	if img.isArtifact {
		return nil
	}

	if len(phase.ImagesToPublish) == 0 {
		return phase.publishImage(img)
	}

	for _, name := range phase.ImagesToPublish {
		if name == img.GetName() {
			return phase.publishImage(img)
		}
	}

	return nil
}

func (phase *PublishImagesPhase) ImageProcessingShouldBeStopped(img *Image) bool {
	return false
}

func (phase *PublishImagesPhase) publishImage(img *Image) error {
	var nonEmptySchemeInOrder []tag_strategy.TagStrategy
	for strategy, tags := range phase.TagsByScheme {
		if len(tags) == 0 {
			continue
		}

		nonEmptySchemeInOrder = append(nonEmptySchemeInOrder, strategy)
	}

	if phase.Conveyor.localGitRepo != nil {
		if err := logboek.Info.LogProcess(fmt.Sprintf("publishing image %s git metadata", img.GetName()), logboek.LevelLogProcessOptions{}, func() error {
			headCommit, err := phase.Conveyor.localGitRepo.HeadCommit()
			if err != nil {
				return err
			}

			if metadata, err := phase.Conveyor.StagesManager.StagesStorage.GetImageMetadataByCommit(phase.Conveyor.projectName(), img.GetName(), headCommit); err != nil {
				return fmt.Errorf("unable to get image %s metadata by commit %s: %s", img.GetName(), headCommit, err)
			} else if metadata != nil {
				if metadata.ContentSignature != img.GetContentSignature() {
					// TODO: Check image existance and automatically allow republish if no images found by this commit. What if multiple images are published by multiple tagging strategies (including custom)?
					// TODO: allowInconsistentPublish: true option for werf.yaml
					// FIXME: return fmt.Errorf("inconsistent build: found already published image with stages-signature %s by commit %s, cannot publish a new image with stages-signature %s by the same commit", metadata.ContentSignature, headCommit, img.GetContentSignature())
					return phase.Conveyor.StagesManager.StagesStorage.PutImageCommit(phase.Conveyor.projectName(), img.GetName(), headCommit, &storage.ImageMetadata{ContentSignature: img.GetContentSignature()})
				}
				return nil
			} else {
				return phase.Conveyor.StagesManager.StagesStorage.PutImageCommit(phase.Conveyor.projectName(), img.GetName(), headCommit, &storage.ImageMetadata{ContentSignature: img.GetContentSignature()})
			}
		}); err != nil {
			return err
		}
	}

	var existingTags []string
	if tags, err := phase.fetchExistingTags(img.GetName()); err != nil {
		return err
	} else {
		existingTags = tags
	}

	for _, strategy := range nonEmptySchemeInOrder {
		imageMetaTags := phase.TagsByScheme[strategy]

		if err := logboek.Info.LogProcess(
			fmt.Sprintf("%s tagging strategy", string(strategy)),
			logboek.LevelLogProcessOptions{Style: logboek.HighlightStyle()},
			func() error {
				for _, imageMetaTag := range imageMetaTags {
					if err := phase.publishImageByTag(img, imageMetaTag, strategy, publishImageByTagOptions{ExistingTagsList: existingTags, CheckAlreadyExistingTagByContentSignatureLabel: true}); err != nil {
						return fmt.Errorf("error publishing image %s by tag %s: %s", img.LogName(), imageMetaTag, err)
					}
				}

				return nil
			},
		); err != nil {
			return err
		}
	}

	if phase.TagByStagesSignature {
		if err := logboek.Info.LogProcess(
			fmt.Sprintf("%s tagging strategy", tag_strategy.StagesSignature),
			logboek.LevelLogProcessOptions{Style: logboek.HighlightStyle()},
			func() error {
				if err := phase.publishImageByTag(img, img.GetContentSignature(), tag_strategy.StagesSignature, publishImageByTagOptions{ExistingTagsList: existingTags}); err != nil {
					return fmt.Errorf("error publishing image %s by image signature %s: %s", img.GetName(), img.GetContentSignature(), err)
				}

				return nil
			},
		); err != nil {
			return err
		}
	}

	return nil
}

func (phase *PublishImagesPhase) fetchExistingTags(imageName string) (existingTags []string, err error) {
	logProcessMsg := fmt.Sprintf("Fetching existing repo tags")
	_ = logboek.Info.LogProcessInline(logProcessMsg, logboek.LevelLogProcessInlineOptions{}, func() error {
		existingTags, err = phase.ImagesRepo.GetAllImageRepoTags(imageName)
		return nil
	})
	logboek.Info.LogOptionalLn()

	if err != nil {
		return existingTags, fmt.Errorf("error fetching existing tags from image repository %s: %s", phase.ImagesRepo.String(), err)
	}
	return existingTags, nil
}

type publishImageByTagOptions struct {
	CheckAlreadyExistingTagByContentSignatureLabel bool
	ExistingTagsList                               []string
}

func (phase *PublishImagesPhase) publishImageByTag(img *Image, imageMetaTag string, tagStrategy tag_strategy.TagStrategy, opts publishImageByTagOptions) error {
	imageRepository := phase.ImagesRepo.ImageRepositoryName(img.GetName())
	imageName := phase.ImagesRepo.ImageRepositoryNameWithTag(img.GetName(), imageMetaTag)
	imageActualTag := phase.ImagesRepo.ImageRepositoryTag(img.GetName(), imageMetaTag)

	alreadyExists, alreadyExistingDockerImageID, err := phase.checkImageAlreadyExists(opts.ExistingTagsList, img.GetName(), imageMetaTag, img.GetContentSignature(), opts.CheckAlreadyExistingTagByContentSignatureLabel)
	if err != nil {
		return fmt.Errorf("error checking image %s already exists in the images repo: %s", img.LogName(), err)
	}

	if alreadyExists {
		logboek.Default.LogFHighlight("%s tag %s is up-to-date\n", string(tagStrategy), imageActualTag)

		_ = logboek.WithIndent(func() error {
			logboek.Default.LogFDetails("images-repo: %s\n", imageRepository)
			logboek.Default.LogFDetails("      image: %s\n", imageName)
			return nil
		})

		logboek.LogOptionalLn()

		phase.PublishReport.Images[img.GetName()] = PublishReportImageRecord{
			WerfImageName: img.GetName(),
			DockerRepo:    imageRepository,
			DockerTag:     imageActualTag,
			DockerImageID: alreadyExistingDockerImageID,
		}

		return nil
	}

	publishImage := container_runtime.NewWerfImage(phase.Conveyor.GetStageImage(img.GetLastNonEmptyStage().GetImage().Name()), imageName, phase.Conveyor.ContainerRuntime.(*container_runtime.LocalDockerServerRuntime))

	publishImage.Container().ServiceCommitChangeOptions().AddLabel(map[string]string{
		image.WerfDockerImageName:       imageName,
		image.WerfTagStrategyLabel:      string(tagStrategy),
		image.WerfImageLabel:            "true",
		image.WerfImageNameLabel:        img.GetName(),
		image.WerfImageTagLabel:         imageMetaTag,
		image.WerfContentSignatureLabel: img.GetContentSignature(),
		image.WerfImageVersionLabel:     image.WerfImageVersion,
	})

	successInfoSectionFunc := func() {
		_ = logboek.WithIndent(func() error {
			logboek.Default.LogFDetails("images-repo: %s\n", imageRepository)
			logboek.Default.LogFDetails("      image: %s\n", imageName)
			return nil
		})
	}

	publishingFunc := func() error {
		if err := phase.Conveyor.StagesManager.FetchStage(img.GetLastNonEmptyStage()); err != nil {
			return err
		}

		if err := logboek.Info.LogProcess("Building final image with meta information", logboek.LevelLogProcessOptions{}, func() error {
			if err := publishImage.Build(container_runtime.BuildOptions{}); err != nil {
				return fmt.Errorf("error building %s with tagging strategy '%s': %s", imageName, tagStrategy, err)
			}
			return nil
		}); err != nil {
			return err
		}

		if lock, err := phase.Conveyor.StorageLockManager.LockImage(phase.Conveyor.projectName(), imageName); err != nil {
			return fmt.Errorf("error locking image %s: %s", imageName, err)
		} else {
			defer phase.Conveyor.StorageLockManager.Unlock(lock)
		}

		existingTags, err := phase.fetchExistingTags(img.GetName())
		if err != nil {
			return err
		}

		alreadyExists, alreadyExistingImageID, err := phase.checkImageAlreadyExists(existingTags, img.GetName(), imageMetaTag, img.GetContentSignature(), opts.CheckAlreadyExistingTagByContentSignatureLabel)
		if err != nil {
			return fmt.Errorf("error checking image %s already exists in the images repo: %s", img.LogName(), err)
		}

		if alreadyExists {
			logboek.Default.LogFHighlight("%s tag %s is up-to-date\n", string(tagStrategy), imageActualTag)
			_ = logboek.WithIndent(func() error {
				logboek.Info.LogFDetails("discarding newly built image %s\n", publishImage.MustGetBuiltId())
				logboek.Default.LogFDetails("images-repo: %s\n", imageRepository)
				logboek.Default.LogFDetails("      image: %s\n", imageName)

				return nil
			})

			logboek.LogOptionalLn()

			phase.PublishReport.Images[img.GetName()] = PublishReportImageRecord{
				WerfImageName: img.GetName(),
				DockerRepo:    imageRepository,
				DockerTag:     imageActualTag,
				DockerImageID: alreadyExistingImageID,
			}

			return nil
		}

		if err := phase.ImagesRepo.PublishImage(publishImage); err != nil {
			return err
		}

		phase.PublishReport.Images[img.GetName()] = PublishReportImageRecord{
			WerfImageName: img.GetName(),
			DockerRepo:    imageRepository,
			DockerTag:     imageActualTag,
			DockerImageID: publishImage.MustGetBuiltId(),
		}

		return nil
	}

	return logboek.Default.LogProcess(
		fmt.Sprintf("Publishing image %s by %s tag %s", img.LogName(), tagStrategy, imageMetaTag),
		logboek.LevelLogProcessOptions{
			SuccessInfoSectionFunc: successInfoSectionFunc,
			Style:                  logboek.HighlightStyle(),
		},
		publishingFunc)
}

func (phase *PublishImagesPhase) checkImageAlreadyExists(existingTags []string, werfImageName, imageMetaTag, imageContentSignature string, checkAlreadyExistingTagByContentSignatureFromLabels bool) (bool, string, error) {
	imageActualTag := phase.ImagesRepo.ImageRepositoryTag(werfImageName, imageMetaTag)

	if !util.IsStringsContainValue(existingTags, imageActualTag) {
		return false, "", nil
	} else if !checkAlreadyExistingTagByContentSignatureFromLabels {
		return true, "", nil
	}

	var repoImageContentSignature string
	var repoDockerImageID string
	var err error
	getImageContentSignature := func() error {
		repoImage, err := phase.ImagesRepo.GetRepoImage(werfImageName, imageMetaTag)
		if err != nil {
			return err
		}
		repoImageContentSignature = repoImage.Labels[image.WerfContentSignatureLabel]
		repoDockerImageID = repoImage.ID
		return nil
	}

	logProcessMsg := fmt.Sprintf("Getting existing tag %s manifest", imageActualTag)
	err = logboek.Info.LogProcessInline(logProcessMsg, logboek.LevelLogProcessInlineOptions{}, getImageContentSignature)
	if err != nil {
		return false, "", fmt.Errorf("unable to get image %s parent id: %s", werfImageName, err)
	}

	logboek.Debug.LogF("Current image content signature: %s\n", imageContentSignature)
	logboek.Debug.LogF("Already published image content signature: %s\n", repoImageContentSignature)
	logboek.Debug.LogF("Already published image docker ID: %s\n", repoDockerImageID)

	return imageContentSignature == repoImageContentSignature, repoDockerImageID, nil
}
