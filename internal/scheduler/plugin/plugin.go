package plugin

import (
	"context"
	"os"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	"github.com/linshanyu/astra-scheduler/internal/scheduler/policy"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kube-scheduler/framework"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const Name = "AstraScheduler"

const (
	NodeResourceNamespaceEnv     = "ASTRA_NODE_RESOURCE_NAMESPACE"
	DefaultNodeResourceNamespace = "astra-system"
)

type AstraScheduler struct {
	handle                framework.Handle
	client                ctrlclient.Client
	policy                *policy.SimplePolicy
	nodeResourceNamespace string
}

var _ framework.FilterPlugin = &AstraScheduler{}
var _ framework.PreFilterPlugin = &AstraScheduler{}
var _ framework.PreScorePlugin = &AstraScheduler{}
var _ framework.ScorePlugin = &AstraScheduler{}
var _ framework.ScoreExtensions = &AstraScheduler{}
var _ framework.ReservePlugin = &AstraScheduler{}

func New(_ context.Context, _ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	scheme := runtime.NewScheme()
	if err := astrav1alpha1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	client, err := ctrlclient.New(handle.KubeConfig(), ctrlclient.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	return &AstraScheduler{
		handle:                handle,
		client:                client,
		policy:                policy.NewSimplePolicy(),
		nodeResourceNamespace: nodeResourceNamespace(),
	}, nil
}

func (p *AstraScheduler) Name() string {
	return Name
}

func nodeResourceNamespace() string {
	if namespace := os.Getenv(NodeResourceNamespaceEnv); namespace != "" {
		return namespace
	}
	return DefaultNodeResourceNamespace
}
