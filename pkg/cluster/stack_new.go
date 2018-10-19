package cluster

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/kubernetes-incubator/kube-aws/credential"
	"github.com/kubernetes-incubator/kube-aws/logger"
	"github.com/kubernetes-incubator/kube-aws/pkg/clusterapi"
	"github.com/kubernetes-incubator/kube-aws/plugin/clusterextension"
)

func newStack(stackName string, conf *Config, opts clusterapi.StackTemplateOptions, assetsConfig *credential.CompactAssets, tmplCtx func(stack *Stack) (interface{}, error)) (*Stack, error) {
	logger.Debugf("Called newStack")

	stack := &Stack{
		StackName:            stackName,
		StackTemplateOptions: opts,
		ClusterName:          conf.ClusterName,
		S3URI:                conf.S3URI,
		Region:               conf.Region,
		AssetsConfig:         assetsConfig,
		Config:               conf,
	}

	ctx, err := tmplCtx(stack)
	if err != nil {
		return nil, err
	}

	stack.tmplCtx = ctx

	return stack, nil
}

func NewControlPlaneStack(conf *Config, opts clusterapi.StackTemplateOptions, extras clusterextension.ClusterExtension, assetsConfig *credential.CompactAssets) (*Stack, error) {
	stack, err := newStack("control-plane", conf, opts, assetsConfig, func(stack *Stack) (interface{}, error) {
		return DefaultTmplCtx{
			Stack:  stack,
			Config: conf,
		}, nil
	})

	if err != nil {
		return nil, err
	}

	extraStack, err := extras.ControlPlaneStack(stack)
	if err != nil {
		return nil, fmt.Errorf("failed to load control-plane stack extras from plugins: %v", err)
	}
	stack.ExtraCfnResources = extraStack.Resources

	extraController, err := extras.Controller(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to load controller node extras from plugins: %v", err)
	}

	stack.archivedFiles = extraController.ArchivedFiles
	stack.CfnInitConfigSets = extraController.CfnInitConfigSets

	if err := stack.RenderAddControllerUserdata(opts); err != nil {
		return nil, err
	}

	assets, err := stack.buildAssets()
	if err != nil {
		return nil, err
	}

	stack.assets = assets

	return stack, nil
}

func NewNetworkStack(conf *Config, opts clusterapi.StackTemplateOptions, extras clusterextension.ClusterExtension, assetsConfig *credential.CompactAssets) (*Stack, error) {
	stack, err := newStack("network", conf, opts, assetsConfig, func(stack *Stack) (interface{}, error) {
		return DefaultTmplCtx{
			Stack:  stack,
			Config: conf,
		}, nil
	})
	if err != nil {
		return nil, err
	}

	assets, err := stack.buildAssets()
	if err != nil {
		return nil, err
	}

	stack.assets = assets

	return stack, nil
}

func NewEtcdStack(conf *Config, opts clusterapi.StackTemplateOptions, extras clusterextension.ClusterExtension, assetsConfig *credential.CompactAssets, session *session.Session) (*Stack, error) {
	stack, err := newStack("etcd", conf, opts, assetsConfig, func(stack *Stack) (interface{}, error) {

		// create the context that will be used to build the assets (combination of config + existing state)
		s := &Session{Session: session}
		existingState, err := s.InspectEtcdExistingState(conf)
		if err != nil {
			return nil, fmt.Errorf("Could not inspect existing etcd state: %v", err)
		}
		return EtcdTmplCtx{
			Stack:             stack,
			Config:            conf,
			EtcdExistingState: existingState,
		}, nil
	})
	if err != nil {
		return nil, err
	}

	extraStack, err := extras.EtcdStack(stack)
	if err != nil {
		return nil, fmt.Errorf("failed to load etcd stack extras from plugins: %v", err)
	}
	stack.ExtraCfnResources = extraStack.Resources

	extraEtcd, err := extras.Etcd()
	if err != nil {
		return nil, fmt.Errorf("failed to load etcd node extras from plugins: %v", err)
	}

	conf.Etcd.CustomSystemdUnits = append(conf.Etcd.CustomSystemdUnits, extraEtcd.SystemdUnits...)
	conf.Etcd.CustomFiles = append(conf.Etcd.CustomFiles, extraEtcd.Files...)
	conf.Etcd.IAMConfig.Policy.Statements = append(conf.Etcd.IAMConfig.Policy.Statements, extraEtcd.IAMPolicyStatements...)
	// TODO Not implemented yet
	//stack.archivedFiles = extraEtcd.ArchivedFiles
	//stack.CfnInitConfigSets = extraEtcd.CfnInitConfigSets

	if err := stack.RenderAddEtcdUserdata(opts); err != nil {
		return nil, err
	}

	assets, err := stack.buildAssets()
	if err != nil {
		return nil, err
	}

	stack.assets = assets

	return stack, nil
}

func NewWorkerStack(conf *Config, npconf *NodePoolConfig, opts clusterapi.StackTemplateOptions, extras clusterextension.ClusterExtension, assetsConfig *credential.CompactAssets) (*Stack, error) {
	stack, err := newStack(npconf.StackName(), conf, opts, assetsConfig, func(stack *Stack) (interface{}, error) {
		return WorkerTmplCtx{
			Stack:          stack,
			NodePoolConfig: npconf,
		}, nil
	})
	if err != nil {
		return nil, err
	}

	stack.NodePoolConfig = npconf

	extraStack, err := extras.NodePoolStack(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to load node pool stack extras from plugins: %v", err)
	}
	stack.ExtraCfnResources = extraStack.Resources

	extraWorker, err := extras.Worker(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to load controller node extras from plugins: %v", err)
	}
	if len(npconf.Kubelet.Kubeconfig) == 0 {
		npconf.Kubelet.Kubeconfig = extraWorker.Kubeconfig
	}
	npconf.Kubelet.Mounts = append(conf.Kubelet.Mounts, extraWorker.KubeletVolumeMounts...)
	npconf.CustomSystemdUnits = append(npconf.CustomSystemdUnits, extraWorker.SystemdUnits...)
	npconf.CustomFiles = append(npconf.CustomFiles, extraWorker.Files...)
	npconf.IAMConfig.Policy.Statements = append(npconf.IAMConfig.Policy.Statements, extraWorker.IAMPolicyStatements...)
	npconf.KubeAWSVersion = VERSION
	if len(npconf.StackTags) == 0 {
		npconf.StackTags = make(map[string]string, 1)
	}
	npconf.StackTags["kube-aws:version"] = VERSION

	for k, v := range extraWorker.NodeLabels {
		npconf.NodeSettings.NodeLabels[k] = v
	}
	for k, v := range extraWorker.FeatureGates {
		npconf.NodeSettings.FeatureGates[k] = v
	}

	stack.archivedFiles = extraWorker.ArchivedFiles
	stack.CfnInitConfigSets = extraWorker.CfnInitConfigSets

	assets, err := stack.buildAssets()
	if err != nil {
		return nil, err
	}

	stack.assets = assets

	return stack, nil
}
