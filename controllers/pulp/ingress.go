/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pulp

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	repomanagerv1alpha1 "github.com/pulp/pulp-operator/api/v1alpha1"
	"github.com/pulp/pulp-operator/controllers"
)

func (r *PulpReconciler) pulpIngressController(ctx context.Context, pulp *repomanagerv1alpha1.Pulp, log logr.Logger) (ctrl.Result, error) {

	podList := &corev1.PodList{}
	labels := map[string]string{
		"app.kubernetes.io/part-of":    pulp.Spec.DeploymentType,
		"app.kubernetes.io/managed-by": pulp.Spec.DeploymentType + "-operator",
		"app.kubernetes.io/instance":   pulp.Spec.DeploymentType + "-content-" + pulp.Name,
		"app.kubernetes.io/component":  "content",
	}
	listOpts := []client.ListOption{
		client.InNamespace(pulp.Namespace),
		client.MatchingLabels(labels),
	}
	if err := r.List(ctx, podList, listOpts...); err != nil {
		log.Error(err, "Failed to list Content pods", "Pulp.Namespace", pulp.Namespace, "Pulp.Name", pulp.Name)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	var IsPodRunning bool = false
	var pod = corev1.Pod{}
	for _, p := range podList.Items {
		log.Info("Checking Content pod", "Pod", p.Name, "Status", p.Status.Phase)
		if p.Status.Phase == "Running" {
			log.Info("Running!", "Pod", p.Name, "Status", p.Status.Phase)
			IsPodRunning = true
			pod = p
			break
		} else {
			log.Info("Content Pod isn't running yet!", "Pod", p.Name, "Status", p.Status.Phase)
		}
	}

	if !IsPodRunning {
		log.Info("Content pod isn't running yet!")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	execCmd := []string{
		"/usr/bin/route_paths.py", pulp.Name,
	}
	cmdOutput, err := controllers.ContainerExec(r, &pod, execCmd, "content", pod.Namespace)
	if err != nil {
		log.Error(err, "Failed to get ingresss from "+pod.Name)
		r.updateStatus(ctx, pulp, metav1.ConditionFalse, pulp.Spec.DeploymentType+"-Ingress-Ready", "Failed to get ingresss!", "FailedGet"+pod.Name)
		return ctrl.Result{}, err
	}
	var pulpPlugins []IngressPlugin
	json.Unmarshal([]byte(cmdOutput), &pulpPlugins)
	defaultPlugins := []IngressPlugin{
		{
			Name:        pulp.Name + "-content",
			Path:        getPulpSetting(pulp, "content_path_prefix"),
			TargetPort:  "content-24816",
			ServiceName: pulp.Name + "-content-svc",
		},
		{
			Name:        pulp.Name + "-api-v3",
			Path:        getPulpSetting(pulp, "api_root") + "api/v3/",
			TargetPort:  "api-24817",
			ServiceName: pulp.Name + "-api-svc",
		},
		{
			Name:        pulp.Name + "-auth",
			Path:        "/auth/login/",
			TargetPort:  "api-24817",
			ServiceName: pulp.Name + "-api-svc",
		},
		{
			Name:        pulp.Name,
			Path:        "/",
			TargetPort:  "api-24817",
			ServiceName: pulp.Name + "-api-svc",
		},
	}
	pulpPlugins = append(defaultPlugins, pulpPlugins...)

	// get ingress
	pulpIngress := &netv1.Ingress{}
	err = r.Get(ctx, types.NamespacedName{Name: pulp.Name, Namespace: pulp.Namespace}, pulpIngress)

	// Create the ingress in case it is not found
	if err != nil && errors.IsNotFound(err) {
		ingressPwd := pulpIngressObject(pulp, pulpPlugins)
		ctrl.SetControllerReference(pulp, ingressPwd, r.Scheme)
		log.Info("Creating a new ingress", "Ingress.Namespace", ingressPwd.Namespace, "Ingress.Name", ingressPwd.Name)
		r.updateStatus(ctx, pulp, metav1.ConditionFalse, pulp.Spec.DeploymentType+"-Ingress-Ready", "CreatingIngress", "Creating "+pulp.Name+"-ingress")
		err = r.Create(ctx, ingressPwd)
		if err != nil {
			log.Error(err, "Failed to create new ingress", "Ingress.Namespace", ingressPwd.Namespace, "Ingress.Name", ingressPwd.Name)
			r.updateStatus(ctx, pulp, metav1.ConditionFalse, pulp.Spec.DeploymentType+"-Ingress-Ready", "ErrorCreatingIngress", "Failed to create "+pulp.Name+"-ingress: "+err.Error())
			r.recorder.Event(pulp, corev1.EventTypeWarning, "Failed", "Failed to create new ingress")
			return ctrl.Result{}, err
		}
	} else if err != nil {
		log.Error(err, "Failed to get ingress")
		return ctrl.Result{}, err
	}

	// we should only update the status when Ingress-Ready==false
	if v1.IsStatusConditionFalse(pulp.Status.Conditions, cases.Title(language.English, cases.Compact).String(pulp.Spec.DeploymentType)+"-Ingress-Ready") {
		r.updateStatus(ctx, pulp, metav1.ConditionTrue, pulp.Spec.DeploymentType+"-Ingress-Ready", "IngressTasksFinished", "All Ingress tasks ran successfully")
		r.recorder.Event(pulp, corev1.EventTypeNormal, "IngressReady", "All Ingress tasks ran successfully")
	}
	return ctrl.Result{}, nil
}

// pulp-ingress
func pulpIngressObject(m *repomanagerv1alpha1.Pulp, plugins []IngressPlugin) *netv1.Ingress {
	annotation := map[string]string{
		"haproxy.router.openshift.io/timeout":               m.Spec.HAProxyTimeout,
		"nginx.ingress.kubernetes.io/proxy-body-size":       "0",
		"nginx.org/client-max-body-size":                    "10m",
		"nginx.ingress.kubernetes.io/proxy-read-timeout":    "120s",
		"nginx.ingress.kubernetes.io/proxy-connect-timeout": "120s",
		"nginx.ingress.kubernetes.io/proxy-send-timeout":    "120s",
	}

	var paths []netv1.HTTPIngressPath
	var path netv1.HTTPIngressPath
	pathType := netv1.PathTypePrefix
	rewrite := ""
	for _, plugin := range plugins {
		if len(plugin.Rewrite) > 0 {
			rewrite = "rewrite ^" + strings.TrimRight(plugin.Path, "/") + "* " + plugin.Rewrite + ";"
			if strings.Contains(annotation["nginx.ingress.kubernetes.io/configuration-snippet"], rewrite) {
				continue
			}
			annotation["nginx.ingress.kubernetes.io/configuration-snippet"] = rewrite
			continue
		}
		path = netv1.HTTPIngressPath{
			Path:     plugin.Path,
			PathType: &pathType,
			Backend: netv1.IngressBackend{
				Service: &netv1.IngressServiceBackend{
					Name: plugin.ServiceName,
					Port: netv1.ServiceBackendPort{
						Name: plugin.TargetPort,
					},
				},
			},
		}
		paths = append(paths, path)
	}

	return &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        m.Name,
			Namespace:   m.Namespace,
			Annotations: annotation,
		},
		Spec: netv1.IngressSpec{
			Rules: []netv1.IngressRule{
				{
					Host: m.Spec.IngressHost,
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: paths,
						},
					},
				},
			},
		},
	}
}

// IngressPlugin defines a plugin ingress.
type IngressPlugin struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	ServiceName string `json:"serviceName"`
	TargetPort  string `json:"targetPort"`
	Rewrite     string `json:"rewrite"`
}
