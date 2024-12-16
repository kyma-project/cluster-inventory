package shoot

import (
	"fmt"

	gardener "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	imv1 "github.com/kyma-project/infrastructure-manager/api/v1"
	"github.com/kyma-project/infrastructure-manager/pkg/config"
	extender2 "github.com/kyma-project/infrastructure-manager/pkg/gardener/shoot/extender"
	"github.com/kyma-project/infrastructure-manager/pkg/gardener/shoot/extender/auditlogs"
	"github.com/kyma-project/infrastructure-manager/pkg/gardener/shoot/extender/extensions"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Extend func(imv1.Runtime, *gardener.Shoot) error

func baseExtenders(cfg config.ConverterConfig) []Extend {
	return []Extend{
		extender2.ExtendWithAnnotations,
		extender2.ExtendWithLabels,
		extender2.ExtendWithSeedSelector,
		extender2.NewOidcExtender(cfg.Kubernetes.DefaultOperatorOidc),
		extender2.ExtendWithCloudProfile,
		extender2.ExtendWithExposureClassName,
		extender2.NewMaintenanceExtender(cfg.Kubernetes.EnableKubernetesVersionAutoUpdate, cfg.Kubernetes.EnableMachineImageVersionAutoUpdate),
	}
}

type Converter struct {
	extenders []Extend
	config    config.ConverterConfig
}

func newConverter(config config.ConverterConfig, extenders ...Extend) Converter {
	return Converter{
		extenders: extenders,
		config:    config,
	}
}

type CreateOpts struct {
	config.ConverterConfig
	auditlogs.AuditLogData
}

type PatchOpts struct {
	config.ConverterConfig
	auditlogs.AuditLogData
	Zones             []string
	ShootK8SVersion   string
	ShootImageName    string
	ShootImageVersion string
	Extensions        []gardener.Extension
	Resources         []gardener.NamedResourceReference
}

func NewConverterCreate(opts CreateOpts) Converter {
	extendersForCreate := baseExtenders(opts.ConverterConfig)

	extendersForCreate = append(extendersForCreate,
		extender2.NewProviderExtenderForCreateOperation(
			opts.Provider.AWS.EnableIMDSv2,
			opts.MachineImage.DefaultName,
			opts.MachineImage.DefaultVersion,
		),
		extender2.NewDNSExtender(opts.DNS.SecretName, opts.DNS.DomainPrefix, opts.DNS.ProviderType),
		extender2.ExtendWithTolerations,
	)

	extendersForCreate = append(extendersForCreate, extensions.NewExtensionsExtenderForCreate(opts.ConverterConfig, opts.AuditLogData))

	extendersForCreate = append(extendersForCreate,
		extender2.NewKubernetesExtender(opts.Kubernetes.DefaultVersion, ""))

	if opts.AuditLogData != (auditlogs.AuditLogData{}) {
		extendersForCreate = append(extendersForCreate,
			auditlogs.NewAuditlogExtenderForCreate(
				opts.AuditLog.PolicyConfigMapName,
				opts.AuditLogData))
	}

	return newConverter(opts.ConverterConfig, extendersForCreate...)
}

func NewConverterPatch(opts PatchOpts) Converter {
	extendersForPatch := baseExtenders(opts.ConverterConfig)

	extendersForPatch = append(extendersForPatch,
		extender2.NewProviderExtenderPatchOperation(
			opts.Provider.AWS.EnableIMDSv2,
			opts.MachineImage.DefaultName,
			opts.MachineImage.DefaultVersion,
			opts.ShootImageName,
			opts.ShootImageVersion,
			opts.Zones))

	extendersForPatch = append(extendersForPatch,
		extensions.NewExtensionsExtenderForPatch(opts.AuditLogData, opts.Extensions),
		extender2.NewResourcesExtenderForPatch(opts.Resources))

	extendersForPatch = append(extendersForPatch, extender2.NewKubernetesExtender(opts.Kubernetes.DefaultVersion, opts.ShootK8SVersion))

	if opts.AuditLogData != (auditlogs.AuditLogData{}) {
		extendersForPatch = append(extendersForPatch,
			auditlogs.NewAuditlogExtenderForPatch(opts.ConverterConfig.AuditLog.PolicyConfigMapName))
	}

	return newConverter(opts.ConverterConfig, extendersForPatch...)
}

func (c Converter) ToShoot(runtime imv1.Runtime) (gardener.Shoot, error) {
	// The original implementation in the Provisioner: https://github.com/kyma-project/control-plane/blob/3dd257826747384479986d5d79eb20f847741aa6/components/provisioner/internal/model/gardener_config.go#L127

	// If you need to enhance the converter please adhere to the following convention:
	// - fields taken directly from Runtime CR must be added in this function
	// - if any logic is needed to be implemented, either enhance existing, or create a new extender

	shoot := gardener.Shoot{
		TypeMeta: v1.TypeMeta{
			Kind:       "Shoot",
			APIVersion: "core.gardener.cloud/v1beta1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      runtime.Spec.Shoot.Name,
			Namespace: fmt.Sprintf("garden-%s", c.config.Gardener.ProjectName),
		},
		Spec: gardener.ShootSpec{
			Purpose:           &runtime.Spec.Shoot.Purpose,
			Region:            runtime.Spec.Shoot.Region,
			SecretBindingName: &runtime.Spec.Shoot.SecretBindingName,
			Networking: &gardener.Networking{
				Type:     runtime.Spec.Shoot.Networking.Type,
				Nodes:    &runtime.Spec.Shoot.Networking.Nodes,
				Pods:     &runtime.Spec.Shoot.Networking.Pods,
				Services: &runtime.Spec.Shoot.Networking.Services,
			},
			ControlPlane: runtime.Spec.Shoot.ControlPlane,
		},
	}

	for _, extend := range c.extenders {
		if err := extend(runtime, &shoot); err != nil {
			return gardener.Shoot{}, err
		}
	}

	return shoot, nil
}
