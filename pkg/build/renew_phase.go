package build

import (
	"errors"
	"fmt"

	imagePkg "github.com/flant/werf/pkg/image"
	"github.com/flant/werf/pkg/lock"
	"github.com/flant/werf/pkg/logger"
)

var (
	ErrConveyorShouldBeReset = errors.New("conveyor should be reset")
)

func NewRenewPhase() *RenewPhase {
	return &RenewPhase{}
}

type RenewPhase struct{}

func (p *RenewPhase) Run(c *Conveyor) error {
	var resErr error

	logProcessOptions := logger.LogProcessOptions{}
	err := logger.LogProcess("Checking invalid stages cache", logProcessOptions, func() error {
		err := p.run(c)

		if isConveyorShouldBeResetError(err) {
			resErr = err
			return nil
		}

		return err
	})

	if err != nil {
		return err
	}

	return resErr
}

func (p *RenewPhase) run(c *Conveyor) error {
	var conveyorShouldBeReset bool
	var acquiredLocks []string

	unlockLock := func() {
		var lockName string
		lockName, acquiredLocks = acquiredLocks[0], acquiredLocks[1:]
		lock.Unlock(lockName)
	}

	unlockLocks := func() {
		for len(acquiredLocks) > 0 {
			unlockLock()
		}
	}

	defer unlockLocks()

	for _, image := range c.imagesInOrder {
		// lock
		for _, stage := range image.GetStages() {
			img := stage.GetImage()
			if !img.IsExists() {
				continue
			}

			imageLockName := imagePkg.ImageLockName(img.Name())
			err := lock.Lock(imageLockName, lock.LockOptions{})
			if err != nil {
				return fmt.Errorf("failed to lock %s: %s", imageLockName, err)
			}

			acquiredLocks = append(acquiredLocks, imageLockName)

			if err := img.SyncDockerState(); err != nil {
				return err
			}
		}

		shouldResetAllNextStages := false
		for _, s := range image.GetStages() {
			img := s.GetImage()
			if img.IsExists() {
				if stageShouldBeReset, err := s.ShouldBeReset(img); err != nil {
					return err
				} else if stageShouldBeReset || shouldResetAllNextStages {
					conveyorShouldBeReset = true

					logger.LogServiceF("Untag %s for %s/%s\n", img.Name(), image.LogName(), s.Name())

					if err := img.Untag(); err != nil {
						return err
					}

					unlockLock()
				}
			} else {
				shouldResetAllNextStages = true
			}
		}
	}

	unlockLocks()

	if conveyorShouldBeReset {
		return ErrConveyorShouldBeReset
	} else {
		return nil
	}
}

func isConveyorShouldBeResetError(err error) bool {
	return err == ErrConveyorShouldBeReset
}
