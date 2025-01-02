package main

import (
	"context"
	"fmt"
	"github.com/gardener/gardener/pkg/apis/core/v1beta1"
	gardener_types "github.com/gardener/gardener/pkg/client/core/clientset/versioned/typed/core/v1beta1"
	"github.com/kyma-project/infrastructure-manager/hack/runtime-migrator-app/internal/backup"
	"github.com/kyma-project/infrastructure-manager/hack/runtime-migrator-app/internal/initialisation"
	"github.com/kyma-project/infrastructure-manager/hack/runtime-migrator-app/internal/restore"
	"github.com/kyma-project/infrastructure-manager/hack/runtime-migrator-app/internal/shoot"
	v12 "k8s.io/api/rbac/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"log/slog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	timeoutK8sOperation = 20 * time.Second
	expirationTime      = 60 * time.Minute
)

type Restore struct {
	shootClient           gardener_types.ShootInterface
	dynamicGardenerClient client.Client
	kcpClient             client.Client
	outputWriter          restore.OutputWriter
	results               restore.Results
	cfg                   initialisation.RestoreConfig
}

const fieldManagerName = "kim"

func NewRestore(cfg initialisation.RestoreConfig, kcpClient client.Client, shootClient gardener_types.ShootInterface, dynamicGardenerClient client.Client) (Restore, error) {
	outputWriter, err := restore.NewOutputWriter(cfg.OutputPath)
	if err != nil {
		return Restore{}, err
	}

	return Restore{
		shootClient:           shootClient,
		dynamicGardenerClient: dynamicGardenerClient,
		kcpClient:             kcpClient,
		outputWriter:          outputWriter,
		results:               restore.NewRestoreResults(outputWriter.NewResultsDir),
		cfg:                   cfg,
	}, err
}

func (r Restore) Do(ctx context.Context, runtimeIDs []string) error {
	listCtx, cancel := context.WithTimeout(ctx, timeoutK8sOperation)
	defer cancel()

	shootList, err := r.shootClient.List(listCtx, v1.ListOptions{})
	if err != nil {
		return err
	}

	restorer := restore.NewRestorer(r.cfg.BackupDir)

	for _, runtimeID := range runtimeIDs {
		currentShoot, err := shoot.Fetch(ctx, shootList, r.shootClient, runtimeID)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to fetch shoot: %v", err)
			r.results.ErrorOccurred(runtimeID, "", errMsg)
			slog.Error(errMsg, "runtimeID", runtimeID)

			continue
		}

		if shoot.IsBeingDeleted(currentShoot) {
			errMsg := fmt.Sprintf("Shoot is being deleted: %v", err)
			r.results.ErrorOccurred(runtimeID, currentShoot.Name, errMsg)
			slog.Error(errMsg, "runtimeID", runtimeID)

			continue
		}

		objectsToRestore, err := restorer.Do(runtimeID, currentShoot.Name)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to restore runtime: %v", err)
			r.results.ErrorOccurred(runtimeID, currentShoot.Name, errMsg)
			slog.Error(errMsg, "runtimeID", runtimeID)

			continue
		}

		if currentShoot.Generation == objectsToRestore.OriginalShoot.Generation {
			slog.Warn("Verify the current state of the system. Shoot was not modified after backup was prepared. Skipping.", "runtimeID", runtimeID)
			r.results.OperationSkipped(runtimeID, currentShoot.Name)

			continue
		}

		if currentShoot.Generation > objectsToRestore.OriginalShoot.Generation+1 {
			slog.Warn("Verify the current state of the system. Restore should be performed manually, as the backup may overwrite more that on change.", "runtimeID", runtimeID)
			r.results.AutomaticRestoreImpossible(runtimeID, currentShoot.Name)

			continue
		}

		if r.cfg.IsDryRun {
			slog.Info("Runtime processed successfully (dry-run)", "runtimeID", runtimeID)
			r.results.OperationSucceeded(runtimeID, currentShoot.Name)

			continue
		}

		err = r.applyResources(ctx, objectsToRestore, runtimeID)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to restore runtime: %v", err)
			r.results.ErrorOccurred(runtimeID, currentShoot.Name, errMsg)
			slog.Error(errMsg, "runtimeID", runtimeID)

			continue
		}

		slog.Info("Runtime restore performed successfully", "runtimeID", runtimeID)
		r.results.OperationSucceeded(runtimeID, currentShoot.Name)
	}

	resultsFile, err := r.outputWriter.SaveRestoreResults(r.results)
	if err != nil {
		return err
	}

	slog.Info(fmt.Sprintf("Restore completed. Successfully restored backups: %d, Failed operations: %d, Skipped backups: %d, ", r.results.Succeeded, r.results.Failed, r.results.Skipped))
	slog.Info(fmt.Sprintf("Restore results saved in: %s", resultsFile))

	return nil
}

func (r Restore) applyResources(ctx context.Context, objectsToRestore backup.RuntimeBackup, runtimeID string) error {
	err := r.applyShoot(ctx, objectsToRestore.ShootToRestore)
	if err != nil {
		return err
	}

	clusterClient, err := initialisation.GetRuntimeClient(ctx, r.kcpClient, runtimeID)
	if err != nil {
		return err
	}

	err = r.applyCRBs(ctx, clusterClient, objectsToRestore.ClusterRoleBindings)
	if err != nil {
		return err
	}

	return nil
}

func (r Restore) applyShoot(ctx context.Context, shoot v1beta1.Shoot) error {
	patchCtx, cancel := context.WithTimeout(ctx, timeoutK8sOperation)
	defer cancel()

	return r.dynamicGardenerClient.Patch(patchCtx, &shoot, client.Apply, &client.PatchOptions{
		FieldManager: fieldManagerName,
		Force:        ptr.To(true),
	})
}

func (r Restore) applyCRBs(ctx context.Context, clusterClient client.Client, crbs []v12.ClusterRoleBinding) error {
	for _, crb := range crbs {
		if err := applyCRB(ctx, crb, clusterClient); err != nil {
			return err
		}
	}

	return nil
}

func applyCRB(ctx context.Context, object v12.ClusterRoleBinding, clusterClient client.Client) error {
	ctx, cancel := context.WithTimeout(ctx, timeoutK8sOperation)
	defer cancel()

	return clusterClient.Update(ctx, &object, &client.UpdateOptions{
		FieldManager: fieldManagerName,
	})
}
