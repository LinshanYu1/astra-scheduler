/*
Copyright 2026.

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

package main

import (
	"flag"
	"os"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	"github.com/linshanyu/astra-scheduler/internal/agent"
	"github.com/linshanyu/astra-scheduler/internal/agent/backend"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(astrav1alpha1.AddToScheme(scheme))
}

func main() {
	var nodeName string
	var nodeProfileNamespace string
	var backendType string
	var fakeResourceConfigMap string
	var fakeResourceConfigMapNamespace string
	var metricsAddr string
	var probeAddr string

	flag.StringVar(&nodeName, "node-name", os.Getenv("NODE_NAME"), "Kubernetes node name served by this agent.")
	flag.StringVar(&nodeProfileNamespace, "node-profile-namespace", "astra-system", "Namespace containing AINodeResourceProfile objects.")
	flag.StringVar(&backendType, "backend", "fake", "Node backend implementation: fake, nvidia, runtime, cri.")
	flag.StringVar(&fakeResourceConfigMap, "fake-resource-configmap", os.Getenv("FAKE_RESOURCE_CONFIGMAP"), "Optional ConfigMap name used by the fake backend to override node resources.")
	flag.StringVar(&fakeResourceConfigMapNamespace, "fake-resource-configmap-namespace", os.Getenv("FAKE_RESOURCE_CONFIGMAP_NAMESPACE"), "Namespace of the fake resource ConfigMap. Defaults to --node-profile-namespace.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metrics endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the health probe endpoint binds to.")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if nodeName == "" {
		setupLog.Error(nil, "Missing node name. Set --node-name or NODE_NAME.")
		os.Exit(1)
	}

	if fakeResourceConfigMapNamespace == "" {
		fakeResourceConfigMapNamespace = nodeProfileNamespace
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
	})
	if err != nil {
		setupLog.Error(err, "Failed to create manager")
		os.Exit(1)
	}

	nodeBackend, err := backend.NewWithOptions(backendType, backend.Options{
		Client:             mgr.GetClient(),
		NodeName:           nodeName,
		ConfigMapName:      fakeResourceConfigMap,
		ConfigMapNamespace: fakeResourceConfigMapNamespace,
	})
	if err != nil {
		setupLog.Error(err, "Failed to create agent backend")
		os.Exit(1)
	}

	reconciler := agent.NewReconciler(agent.Config{
		NodeName:                nodeName,
		NodeProfileNamespace:    nodeProfileNamespace,
		FakeResourceConfigMap:   fakeResourceConfigMap,
		FakeResourceConfigMapNS: fakeResourceConfigMapNamespace,
	}, mgr.GetClient(), nodeBackend)

	if err := reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to set up agent controller")
		os.Exit(1)
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting Astra agent", "nodeName", nodeName, "backend", backendType, "fakeResourceConfigMap", fakeResourceConfigMap)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Failed to run agent")
		os.Exit(1)
	}
}
