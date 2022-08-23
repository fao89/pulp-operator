package pulp

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/api/meta"

	routev1 "github.com/openshift/api/route/v1"
	repomanagerv1alpha1 "github.com/pulp/pulp-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	PulpName      = "pulp-operator"
	PulpNamespace = "default"
	StsName       = "pulp-operator-database"
	OperatorType  = "pulp"

	timeout  = time.Second * 10
	duration = time.Second * 10
	interval = time.Millisecond * 250
)

var _ = Describe("Pulp controller", Ordered, func() {

	format.MaxLength = 0

	// this is the example pulp CR
	pulp := &repomanagerv1alpha1.Pulp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PulpName,
			Namespace: PulpNamespace,
		},
		Spec: repomanagerv1alpha1.PulpSpec{
			DeploymentType: OperatorType,
			IsK8s:          true,
			CacheEnabled:   true,
			ImageVersion:   "latest",
			Api: repomanagerv1alpha1.Api{
				Replicas: 1,
			},
			Content: repomanagerv1alpha1.Content{
				Replicas: 1,
			},
			Worker: repomanagerv1alpha1.Worker{
				Replicas: 1,
			},
			Web: repomanagerv1alpha1.Web{
				Replicas: 1,
			},
			Database: repomanagerv1alpha1.Database{
				PostgresStorageRequirements: "5Gi",
			},
			FileStorageAccessMode: "ReadWriteOnce",
			FileStorageSize:       "2Gi",
			FileStorageClass:      "standard",
			RedisStorageClass:     "standard",
			IngressType:           "nodeport",
			PulpSettings: repomanagerv1alpha1.PulpSettings{
				ApiRoot: "/pulp/",
			},
		},
	}

	labels := map[string]string{
		"app.kubernetes.io/name":       "postgres",
		"app.kubernetes.io/instance":   "postgres-" + PulpName,
		"app.kubernetes.io/component":  "database",
		"app.kubernetes.io/part-of":    OperatorType,
		"app.kubernetes.io/managed-by": OperatorType + "-operator",
		"owner":                        "pulp-dev",
		"app":                          "postgresql",
		"pulp_cr":                      PulpName,
	}

	replicas := int32(1)

	envVars := []corev1.EnvVar{
		{
			Name: "POSTGRESQL_DATABASE",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: PulpName + "-postgres-configuration",
					},
					Key: "database",
				},
			},
		},
		{
			Name: "POSTGRESQL_USER",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: PulpName + "-postgres-configuration",
					},
					Key: "username",
				},
			},
		},
		{
			Name: "POSTGRESQL_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: PulpName + "-postgres-configuration",
					},
					Key: "password",
				},
			},
		},
		{
			Name: "POSTGRES_DB",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: PulpName + "-postgres-configuration",
					},
					Key: "database",
				},
			},
		},
		{
			Name: "POSTGRES_USER",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: PulpName + "-postgres-configuration",
					},
					Key: "username",
				},
			},
		},
		{
			Name: "POSTGRES_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: PulpName + "-postgres-configuration",
					},
					Key: "password",
				},
			},
		},
		{Name: "PGDATA", Value: "/var/lib/postgresql/data/pgdata"},
		{Name: "POSTGRES_INITDB_ARGS", Value: "--auth-host=scram-sha-256"},
		{Name: "POSTGRES_HOST_AUTH_METHOD", Value: "scram-sha-256"},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "postgres",
			MountPath: filepath.Dir("/var/lib/postgresql/data/pgdata"),
			SubPath:   filepath.Base("/var/lib/postgresql/data/pgdata"),
		},
	}

	livenessProbe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"/bin/sh",
					"-i",
					"-c",
					"pg_isready -U " + OperatorType + " -h 127.0.0.1 -p 5432",
				},
			},
		},
		InitialDelaySeconds: 30,
		PeriodSeconds:       10,
		TimeoutSeconds:      5,
		FailureThreshold:    6,
		SuccessThreshold:    1,
	}

	readinessProbe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"/bin/sh",
					"-i",
					"-c",
					"pg_isready -U " + OperatorType + " -h 127.0.0.1 -p 5432",
				},
			},
		},
		InitialDelaySeconds: 5,
		PeriodSeconds:       10,
		TimeoutSeconds:      5,
		FailureThreshold:    6,
		SuccessThreshold:    1,
	}

	volumeClaimTemplate := []corev1.PersistentVolumeClaim{{
		ObjectMeta: metav1.ObjectMeta{
			Name: "postgres",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("5Gi"),
				},
			},
		},
	}}

	// this is the expected database statefulset that should be
	// provisioned by pulp controller
	expectedSts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      StsName,
			Namespace: PulpNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "postgres",
				"app.kubernetes.io/instance":   "postgres-" + PulpName,
				"app.kubernetes.io/component":  "database",
				"app.kubernetes.io/part-of":    OperatorType,
				"app.kubernetes.io/managed-by": OperatorType + "-operator",
				"owner":                        "pulp-dev",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Affinity:           &corev1.Affinity{},
					ServiceAccountName: "pulp-operator-controller-manager",
					Containers: []corev1.Container{{
						Image: "postgres:13",
						Name:  "postgres",
						Env:   envVars,
						Ports: []corev1.ContainerPort{{
							ContainerPort: int32(5432),
							Name:          "postgres",
							Protocol:      corev1.ProtocolTCP,
						}},
						LivenessProbe:  livenessProbe,
						ReadinessProbe: readinessProbe,
						VolumeMounts:   volumeMounts,
						Resources:      corev1.ResourceRequirements{},
					}},
					RestartPolicy: corev1.RestartPolicyAlways,
				},
			},
			VolumeClaimTemplates: volumeClaimTemplate,
		},
	}
	createdPulp := &repomanagerv1alpha1.Pulp{}
	createdSts := &appsv1.StatefulSet{}

	Context("When creating a Database statefulset", func() {
		It("Should follow the spec from pulp CR", func() {
			ctx := context.Background()

			// test should fail if Pulp CR is not created
			By("Checking Pulp CR instance creation")
			Expect(k8sClient.Create(ctx, pulp)).Should(Succeed())

			// test should fail if Pulp CR is not found
			By("Checking Pulp CR being present")
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: PulpName, Namespace: PulpNamespace}, createdPulp); err != nil {
					fmt.Println(err)
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// test should fail if sts is not found
			By("Checking sts being found")
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: StsName, Namespace: PulpNamespace}, createdSts); err != nil {
					fmt.Println(err)
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// using DeepDerivative to ignore comparison of unset fields from "expectedSts"
			// that are present on "predicate"
			var isEqual = func(predicate interface{}) bool {
				return equality.Semantic.DeepDerivative(expectedSts.Spec.Template, predicate)
			}

			waitPulpOperatorFinish(createdPulp)

			// test should fail if sts is not with the desired spec
			By("Checking sts expected Name")
			Expect(createdSts.Name).Should(Equal(expectedSts.Name))
			By("Checking sts expected Labels")
			Expect(createdSts.Labels).Should(Equal(expectedSts.Labels))
			By("Checking sts expected Replicas")
			Expect(createdSts.Spec.Replicas).Should(Equal(expectedSts.Spec.Replicas))
			By("Checking sts expected Selector")
			Expect(createdSts.Spec.Selector).Should(Equal(expectedSts.Spec.Selector))
			By("Checking sts expected Template")
			Expect(createdSts.Spec.Template).Should(Satisfy(isEqual))
		})
	})

	Context("When updating Database statefulset", func() {
		It("Should be reconciled with what is defined in pulp CR", func() {
			By("Modifying the number of replicas")

			// make sure that there is no tasks running before proceeding
			waitPulpOperatorFinish(createdPulp)

			replicas := int32(3)
			createdSts.Spec.Replicas = &replicas
			k8sClient.Update(ctx, createdSts)

			waitPulpOperatorFinish(createdPulp)

			// request createdSts state to kube-api
			k8sClient.Get(ctx, types.NamespacedName{Name: StsName, Namespace: PulpNamespace}, createdSts)

			// we expect that pulp controller rollback createdSts.spec.replicas to 1
			Expect(createdSts.Spec.Replicas).Should(Equal(expectedSts.Spec.Replicas))

			By("Modifying the container image name")
			newName := "mysql:latest"
			createdSts.Spec.Template.Spec.Containers[0].Image = newName
			k8sClient.Update(ctx, createdSts)

			waitPulpOperatorFinish(createdPulp)

			// request createdSts state to kube-api
			k8sClient.Get(ctx, types.NamespacedName{Name: StsName, Namespace: PulpNamespace}, createdSts)

			// we expect that pulp controller rollback the container image
			Expect(createdSts.Spec.Template.Spec.Containers[0].Image).Should(Equal(expectedSts.Spec.Template.Spec.Containers[0].Image))
		})
	})

	Context("When updating pulp CR", func() {
		It("Should reconcile the database sts", func() {
			By("Modifying database image")

			// make sure that there is no tasks running before proceeding
			waitPulpOperatorFinish(createdPulp)

			createdPulp.Spec.Database.PostgresImage = "postgres:12"
			if err := k8sClient.Update(ctx, createdPulp); err != nil {
				fmt.Println("Error trying to update pulp: ", err)
			}

			waitPulpOperatorFinish(createdPulp)

			// request createdSts state to kube-api
			k8sClient.Get(ctx, types.NamespacedName{Name: StsName, Namespace: PulpNamespace}, createdSts)

			// we expect that pulp controller update sts with the new image defined in pulp CR
			Expect(createdSts.Spec.Template.Spec.Containers[0].Image).Should(Equal("postgres:12"))

		})
	})

	Context("When ingress_type is defined as route", func() {
		It("Should not deploy pulp-web resources and still expose services", func() {

			if strings.ToLower(pulp.Spec.IngressType) != "route" {
				Skip("IngressType != route")
			}

			// make sure that there is no tasks running before proceeding
			waitPulpOperatorFinish(createdPulp)

			By("Creating the default root route path")

			routeName := pulp.Name
			expectedRoutes := make(map[string]interface{})
			expectedRoutes[routeName] = struct {
				Path, TargetPort, ServiceName string
			}{"/", "api-24817", pulp.Name + "-api-svc"}

			route := &routev1.Route{}
			k8sClient.Get(ctx, types.NamespacedName{Name: routeName, Namespace: PulpNamespace}, route)
			Expect(route.Spec.Host).Should(Equal(pulp.Spec.RouteHost))
			Expect(route.Spec.Path).Should(Equal(expectedRoutes[routeName].(struct{ Path string }).Path))
			Expect(route.Spec.Port.TargetPort).Should(Equal(expectedRoutes[routeName].(struct{ TargetPort string }).TargetPort))
			Expect(route.Spec.To.Name).Should(Equal(expectedRoutes[routeName].(struct{ ServiceName string }).ServiceName))

			By("Creating the default content route path")
			routeName = pulp.Name + "-content"
			expectedRoutes[routeName] = struct {
				Path, TargetPort, ServiceName string
			}{"/pulp/content/", "api-24816", pulp.Name + "-content-svc"}

			k8sClient.Get(ctx, types.NamespacedName{Name: routeName, Namespace: PulpNamespace}, route)
			Expect(route.Spec.Host).Should(Equal(pulp.Spec.RouteHost))
			Expect(route.Spec.Path).Should(Equal(expectedRoutes[routeName].(struct{ Path string }).Path))
			Expect(route.Spec.Port.TargetPort).Should(Equal(expectedRoutes[routeName].(struct{ TargetPort string }).TargetPort))
			Expect(route.Spec.To.Name).Should(Equal(expectedRoutes[routeName].(struct{ ServiceName string }).ServiceName))

			By("Creating the default api-v3 route path")
			routeName = pulp.Name + "-api-v3"
			expectedRoutes[routeName] = struct {
				Path, TargetPort, ServiceName string
			}{"/pulp/api/v3", "api-24817", pulp.Name + "-api-svc"}

			k8sClient.Get(ctx, types.NamespacedName{Name: routeName, Namespace: PulpNamespace}, route)
			Expect(route.Spec.Host).Should(Equal(pulp.Spec.RouteHost))
			Expect(route.Spec.Path).Should(Equal(expectedRoutes[routeName].(struct{ Path string }).Path))
			Expect(route.Spec.Port.TargetPort).Should(Equal(expectedRoutes[routeName].(struct{ TargetPort string }).TargetPort))
			Expect(route.Spec.To.Name).Should(Equal(expectedRoutes[routeName].(struct{ ServiceName string }).ServiceName))

			By("Creating the default auth route path")
			routeName = pulp.Name + "-auth"
			expectedRoutes[routeName] = struct {
				Path, TargetPort, ServiceName string
			}{"/auth/login", "api-24817", pulp.Name + "-api-svc"}

			k8sClient.Get(ctx, types.NamespacedName{Name: routeName, Namespace: PulpNamespace}, route)
			Expect(route.Spec.Host).Should(Equal(pulp.Spec.RouteHost))
			Expect(route.Spec.Path).Should(Equal(expectedRoutes[routeName].(struct{ Path string }).Path))
			Expect(route.Spec.Port.TargetPort).Should(Equal(expectedRoutes[routeName].(struct{ TargetPort string }).TargetPort))
			Expect(route.Spec.To.Name).Should(Equal(expectedRoutes[routeName].(struct{ ServiceName string }).ServiceName))

			By("Making sure no deployment/pulp-web is provisioned")
			webDeployment := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: PulpName + "-web", Namespace: PulpNamespace}, webDeployment)
			Expect(err).ShouldNot(BeEmpty())
			Expect(errors.IsNotFound(err)).Should(BeTrue())

			By("Making sure no svc/pulp-web is provisioned")
			webSvc := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: PulpName + "-web-svc", Namespace: PulpNamespace}, webSvc)
			Expect(err).ShouldNot(BeEmpty())
			Expect(errors.IsNotFound(err)).Should(BeTrue())

		})
	})

	Context("When pulp.Spec.Database.PostgresStorageClass and cluster SC are not defined", func() {
		It("Should configure the database pod template with an emptyDir volume", func() {

			By("Making sure that database type is not external")
			if reflect.DeepEqual(pulp.Spec.Database.ExternalDB, repomanagerv1alpha1.ExternalDB{}) {
				Skip("External database does not need to provision a Persistent Volume")
			}

			By("Checking if postgressc is not defined")
			nilPostgresSC := ""
			if pulp.Spec.Database.PostgresStorageClass != &nilPostgresSC {
				Skip("PostgresSC defined")
			}

			By("Checking if there is no default SC")
			if isDefaultSCDefined() {
				Skip("Default storage class defined")
			}

			By("Checking if sts template is configured to use emptyDir volume")
			var found bool
			for _, volume := range createdSts.Spec.Template.Spec.Volumes {
				if volume.Name == "postgres" && reflect.DeepEqual(volume.VolumeSource.EmptyDir, &corev1.EmptyDirVolumeSource{}) {
					found = true
					break
				}
			}
			Expect(found).Should(BeTrue())
		})
	})

	Context("When pulp is not configured with object storage nor pulp.Spec.FileStorageClass is defined and there is no default SC", func() {
		It("Shoud configure the api pod template with an emptyDir volume", func() {
			By("Checking if an object storage is not defined")
			if len(pulp.Spec.ObjectStorageAzureSecret) != 0 || len(pulp.Spec.ObjectStorageS3Secret) != 0 {
				Skip("Object storage defined")
			}

			By("Checking if fileSC is not defined")
			if pulp.Spec.FileStorageClass != "" {
				Skip("FileStorageClass defined")
			}

			By("Checking if there is no default SC")
			if isDefaultSCDefined() {
				Skip("Default storage class defined")
			}

			By("Checking if api deployment is configured to use emptyDir volume")
			var foundTmp, foundAsset bool
			apiDeployment := &appsv1.Deployment{}
			k8sClient.Get(ctx, types.NamespacedName{Name: PulpName + "-api", Namespace: PulpNamespace}, apiDeployment)
			for _, volume := range apiDeployment.Spec.Template.Spec.Volumes {
				if volume.Name == "tmp-file-storage" && reflect.DeepEqual(volume.VolumeSource.EmptyDir, &corev1.EmptyDirVolumeSource{}) {
					foundTmp = true
				}
				if volume.Name == "assets-file-storage" && reflect.DeepEqual(volume.VolumeSource.EmptyDir, &corev1.EmptyDirVolumeSource{}) {
					foundAsset = true
				}
			}
			Expect(foundTmp).Should(BeTrue())
			Expect(foundAsset).Should(BeTrue())
		})
	})

})

// waitPulpOperatorFinish waits until find "Pulp-Operator-Finished-Execution" pulp.Status.Condition
// or 60 seconds timeout
func waitPulpOperatorFinish(createdPulp *repomanagerv1alpha1.Pulp) {
	for timeout := 0; timeout < 60; timeout++ {
		k8sClient.Get(ctx, types.NamespacedName{Name: PulpName, Namespace: PulpNamespace}, createdPulp)
		//a, _ := json.MarshalIndent(createdPulp.Status.Conditions, "", "  ")
		//fmt.Println(string(a))
		if v1.IsStatusConditionTrue(createdPulp.Status.Conditions, "Pulp-Operator-Finished-Execution") {
			// [TODO] For some reason, even after the controller considering that the execution was finished,
			// during a small period some resources were still in update process. I need to investigate
			// this further.
			time.Sleep(time.Millisecond * 100)
			break
		}
		time.Sleep(time.Second)
	}
}

// isDefaultSCDefined returns true if found a StorageClass marked as default
func isDefaultSCDefined() bool {
	scList := &storagev1.StorageClassList{}
	k8sClient.List(ctx, scList)
	for _, sc := range scList.Items {
		annotation := sc.ObjectMeta.GetAnnotations()
		if _, found := annotation["storageclass.kubernetes.io/is-default-class"]; found {
			return true
		}
	}
	return false
}
