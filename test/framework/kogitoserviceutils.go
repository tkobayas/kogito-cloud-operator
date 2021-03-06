// Copyright 2020 Red Hat, Inc. and/or its affiliates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package framework

import (
	"fmt"

	"github.com/kiegroup/kogito-cloud-operator/pkg/apis/app/v1alpha1"
	"github.com/kiegroup/kogito-cloud-operator/pkg/client/kubernetes"
	"github.com/kiegroup/kogito-cloud-operator/pkg/framework"
	"github.com/kiegroup/kogito-cloud-operator/pkg/infrastructure"
	"github.com/kiegroup/kogito-cloud-operator/test/config"
	"github.com/kiegroup/kogito-cloud-operator/test/framework/mappers"
	bddtypes "github.com/kiegroup/kogito-cloud-operator/test/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InstallService install the Kogito Service component
func InstallService(serviceHolder *bddtypes.KogitoServiceHolder, installerType InstallerType, cliDeploymentName string) error {
	return installOrDeployService(serviceHolder, installerType, "install", cliDeploymentName)
}

// DeployService deploy the Kogito Service component
func DeployService(serviceHolder *bddtypes.KogitoServiceHolder, installerType InstallerType) error {
	return installOrDeployService(serviceHolder, installerType, "deploy", serviceHolder.GetName())
}

// InstallOrDeployService the Kogito Service component
func installOrDeployService(serviceHolder *bddtypes.KogitoServiceHolder, installerType InstallerType, cliDeployCommand, cliDeploymentName string) error {
	GetLogger(serviceHolder.GetNamespace()).Infof("%s install %s with %d replicas", serviceHolder.GetName(), installerType, *serviceHolder.GetSpec().GetReplicas())
	var err error
	switch installerType {
	case CLIInstallerType:
		err = cliInstall(serviceHolder, cliDeployCommand, cliDeploymentName)
	case CRInstallerType:
		enableUseKogitoInfraIfInfinispanURLsNotDefined(serviceHolder)
		enableUseKogitoInfraIfKafkaURLsNotDefined(serviceHolder)
		err = crInstall(serviceHolder)
	default:
		panic(fmt.Errorf("Unknown installer type %s", installerType))
	}

	if err == nil {
		err = OnKogitoServiceDeployed(serviceHolder.GetNamespace(), serviceHolder)
	}

	return err
}

// enableUseKogitoInfraIfInfinispanURLsNotDefined sets Kogito service to use KogitoInfra in case persistence is enabled and Infinispan URL is not defined
func enableUseKogitoInfraIfInfinispanURLsNotDefined(serviceHolder *bddtypes.KogitoServiceHolder) {
	if infinispanAware, ok := serviceHolder.GetSpec().(v1alpha1.InfinispanAware); ok {
		if serviceHolder.EnablePersistence && len(infinispanAware.GetInfinispanProperties().URI) == 0 {
			infinispanAware.GetInfinispanProperties().UseKogitoInfra = true
		}
	}
}

// enableUseKogitoInfraIfKafkaURLsNotDefined sets Kogito service to use KogitoInfra in case events are enabled and Kafka URL is not defined
func enableUseKogitoInfraIfKafkaURLsNotDefined(serviceHolder *bddtypes.KogitoServiceHolder) {
	if kafkaAware, ok := serviceHolder.GetSpec().(v1alpha1.KafkaAware); ok {
		if serviceHolder.EnableEvents && (len(kafkaAware.GetKafkaProperties().ExternalURI) == 0 && len(kafkaAware.GetKafkaProperties().Instance) == 0) {
			kafkaAware.GetKafkaProperties().UseKogitoInfra = true
		}
	}
}

// WaitForService waits that the service has a certain number of replicas
func WaitForService(namespace string, serviceName string, replicas int, timeoutInMin int) error {
	return WaitForOnOpenshift(namespace, serviceName+" running", timeoutInMin,
		func() (bool, error) {
			deployment, err := GetDeployment(namespace, serviceName)
			if err != nil {
				return false, err
			}
			if deployment == nil {
				return false, nil
			}
			return deployment.Status.Replicas == int32(replicas) && deployment.Status.AvailableReplicas == int32(replicas), nil
		})
}

// NewObjectMetadata creates a new Object Metadata object.
func NewObjectMetadata(namespace string, name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
	}
}

// NewKogitoServiceSpec creates a new Kogito Service Spec object.
func NewKogitoServiceSpec(replicas int32, fullImage string, defaultImageName string) v1alpha1.KogitoServiceSpec {
	return v1alpha1.KogitoServiceSpec{
		Replicas: &replicas,
		Image:    NewImageOrDefault(fullImage, defaultImageName),
		// Sets insecure image registry as service images can be stored in insecure registries
		InsecureImageRegistry: true,
	}
}

// NewKogitoServiceStatus creates a new Kogito Service Status object.
func NewKogitoServiceStatus() v1alpha1.KogitoServiceStatus {
	return v1alpha1.KogitoServiceStatus{
		ConditionsMeta: v1alpha1.ConditionsMeta{
			Conditions: []v1alpha1.Condition{},
		},
	}
}

// NewImageOrDefault Returns Image parsed from provided image tag or created from configuration options
func NewImageOrDefault(fullImage string, defaultImageName string) string {
	if len(fullImage) > 0 {
		return fullImage
	}

	image := v1alpha1.Image{}
	if isRuntimeImageInformationSet() {

		image.Domain = config.GetServicesImageRegistry()
		image.Namespace = config.GetServicesImageNamespace()
		image.Name = defaultImageName
		image.Tag = config.GetServicesImageVersion()

		if len(image.Domain) == 0 {
			image.Domain = infrastructure.DefaultImageRegistry
		}

		if len(image.Namespace) == 0 {
			image.Namespace = infrastructure.DefaultImageNamespace
		}

		if len(image.Tag) == 0 {
			image.Tag = infrastructure.GetKogitoImageVersion()
		}

		// Update image name with suffix if provided
		if len(config.GetServicesImageNameSuffix()) > 0 {
			image.Name = fmt.Sprintf("%s-%s", image.Name, config.GetServicesImageNameSuffix())
		}
	}
	return framework.ConvertImageToImageTag(image)
}

func isRuntimeImageInformationSet() bool {
	return len(config.GetServicesImageRegistry()) > 0 ||
		len(config.GetServicesImageNamespace()) > 0 ||
		len(config.GetServicesImageNameSuffix()) > 0 ||
		len(config.GetServicesImageVersion()) > 0
}

func crInstall(serviceHolder *bddtypes.KogitoServiceHolder) error {
	if _, err := kubernetes.ResourceC(kubeClient).CreateIfNotExists(serviceHolder.KogitoService); err != nil {
		return fmt.Errorf("Error creating service: %v", err)
	}
	return nil
}

func cliInstall(serviceHolder *bddtypes.KogitoServiceHolder, cliDeployCommand, cliDeploymentName string) error {
	cmd := []string{cliDeployCommand, cliDeploymentName}
	cmd = append(cmd, mappers.GetServiceCLIFlags(serviceHolder)...)

	_, err := ExecuteCliCommandInNamespace(serviceHolder.GetNamespace(), cmd...)
	return err
}

// OnKogitoServiceDeployed is called when a service deployed.
func OnKogitoServiceDeployed(namespace string, service v1alpha1.KogitoService) error {
	if !IsOpenshift() {
		return ExposeServiceOnKubernetes(namespace, service)
	}

	return nil
}
