package cluster

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/kubernetes-incubator/kube-aws/coreos/amiregistry"
	"github.com/kubernetes-incubator/kube-aws/credential"
	"github.com/kubernetes-incubator/kube-aws/logger"
	"github.com/kubernetes-incubator/kube-aws/pkg/clusterapi"
	"github.com/kubernetes-incubator/kube-aws/plugin/clusterextension"
)

func newStack(stackName string, conf *Config, opts clusterapi.StackTemplateOptions, assetsConfig *credential.CompactAssets, tmplCtx func(stack *Stack) (interface{}, error), init func(stack *Stack) error) (*Stack, error) {
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

	if err := init(stack); err != nil {
		return nil, err
	}

	assets, err := stack.buildAssets()
	if err != nil {
		return nil, err
	}
	stack.assets = assets

	return stack, nil
}

func NewControlPlaneStack(conf *Config, opts clusterapi.StackTemplateOptions, extras clusterextension.ClusterExtension, assetsConfig *credential.CompactAssets) (*Stack, error) {
	return newStack(
		ControlPlaneStackName,
		conf,
		opts,
		assetsConfig,
		func(stack *Stack) (interface{}, error) {
			return ControllerTmplCtx{
				Stack:  stack,
				Config: conf,
			}, nil
		},
		func(stack *Stack) error {
			extraStack, err := extras.ControlPlaneStack(stack)
			if err != nil {
				return fmt.Errorf("failed to load control-plane stack extras from plugins: %v", err)
			}
			stack.ExtraCfnResources = extraStack.Resources

			extraController, err := extras.Controller(conf)
			if err != nil {
				return fmt.Errorf("failed to load controller node extras from plugins: %v", err)
			}

			stack.archivedFiles = extraController.ArchivedFiles
			stack.CfnInitConfigSets = extraController.CfnInitConfigSets

			return stack.RenderAddControllerUserdata(opts)
		},
	)
}

func NewNetworkStack(conf *Config, nodePools []*Stack, opts clusterapi.StackTemplateOptions, extras clusterextension.ClusterExtension, assetsConfig *credential.CompactAssets) (*Stack, error) {
	return newStack(
		"network",
		conf,
		opts,
		assetsConfig,
		func(stack *Stack) (interface{}, error) {
			nps := []WorkerTmplCtx{}
			for _, s := range nodePools {
				nps = append(nps, s.tmplCtx.(WorkerTmplCtx))
			}

			return NetworkTmplCtx{
				Stack:           stack,
				Config:          conf,
				WorkerNodePools: nps,
			}, nil
		},
		func(stack *Stack) error {
			return nil
		},
	)
}

func NewEtcdStack(conf *Config, opts clusterapi.StackTemplateOptions, extras clusterextension.ClusterExtension, assetsConfig *credential.CompactAssets, session *session.Session) (*Stack, error) {
	return newStack(
		"etcd",
		conf,
		opts,
		assetsConfig,
		func(stack *Stack) (interface{}, error) {
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
		}, func(stack *Stack) error {
			extraStack, err := extras.EtcdStack(stack)
			if err != nil {
				return fmt.Errorf("failed to load etcd stack extras from plugins: %v", err)
			}
			stack.ExtraCfnResources = extraStack.Resources

			extraEtcd, err := extras.Etcd()
			if err != nil {
				return fmt.Errorf("failed to load etcd node extras from plugins: %v", err)
			}

			conf.Etcd.CustomSystemdUnits = append(conf.Etcd.CustomSystemdUnits, extraEtcd.SystemdUnits...)
			conf.Etcd.CustomFiles = append(conf.Etcd.CustomFiles, extraEtcd.Files...)
			conf.Etcd.IAMConfig.Policy.Statements = append(conf.Etcd.IAMConfig.Policy.Statements, extraEtcd.IAMPolicyStatements...)
			// TODO Not implemented yet
			//stack.archivedFiles = extraEtcd.ArchivedFiles
			//stack.CfnInitConfigSets = extraEtcd.CfnInitConfigSets

			return stack.RenderAddEtcdUserdata(opts)
		},
	)
}

func NewWorkerStack(conf *Config, npconf *NodePoolConfig, opts clusterapi.StackTemplateOptions, extras clusterextension.ClusterExtension, assetsConfig *credential.CompactAssets) (*Stack, error) {
	return newStack(
		npconf.StackName(),
		conf,
		opts,
		assetsConfig,
		func(stack *Stack) (interface{}, error) {
			var ami string
			if npconf.AmiId == "" {
				var err error
				if ami, err = amiregistry.GetAMI(conf.Region.String(), npconf.ReleaseChannel); err != nil {
					return nil, fmt.Errorf("failed getting AMI for config: %v", err)
				}
			} else {
				ami = npconf.AmiId
			}

			return WorkerTmplCtx{
				Stack:          stack,
				NodePoolConfig: npconf,
				AMI:            ami,
			}, nil
		},
		func(stack *Stack) error {
			stack.NodePoolConfig = npconf

			extraStack, err := extras.NodePoolStack(conf)
			if err != nil {
				return fmt.Errorf("failed to load node pool stack extras from plugins: %v", err)
			}
			stack.ExtraCfnResources = extraStack.Resources

			extraWorker, err := extras.Worker(conf)
			if err != nil {
				return fmt.Errorf("failed to load controller node extras from plugins: %v", err)
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

			return stack.RenderAddWorkerUserdata(opts)
		},
	)
}
