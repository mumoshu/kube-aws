package clusterextension

import (
	"fmt"

	"encoding/json"
	"github.com/kubernetes-incubator/kube-aws/model"
	"github.com/kubernetes-incubator/kube-aws/pki"
	"github.com/kubernetes-incubator/kube-aws/plugin/plugincontents"
	"github.com/kubernetes-incubator/kube-aws/plugin/pluginmodel"
	"github.com/kubernetes-incubator/kube-aws/plugin/pluginutil"
	"github.com/kubernetes-incubator/kube-aws/provisioner"
	"github.com/kubernetes-incubator/kube-aws/tmpl"
	"os"
)

type ClusterExtension struct {
	plugins []*pluginmodel.Plugin
	configs model.PluginConfigs
	config  interface{}
}

func NewExtrasFromPlugins(plugins []*pluginmodel.Plugin, configs model.PluginConfigs, config interface{}) ClusterExtension {
	return ClusterExtension{
		plugins: plugins,
		configs: configs,
		config:  config,
	}
}

type stack struct {
	Resources map[string]interface{}
}

func (e ClusterExtension) KeyPairSpecs() ([]pki.KeyPairSpec, error) {
	keypairs := []pki.KeyPairSpec{}
	err := e.foreachEnabledPlugins(func(p *pluginmodel.Plugin, pc *model.PluginConfig) error {
		for _, spec := range p.PKI.KeyPairs {
			keypairs = append(keypairs, spec)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return keypairs, nil
}

func (e ClusterExtension) RootStack() (*stack, error) {
	return e.stackExt("root", func(p *pluginmodel.Plugin) pluginmodel.Stack {
		return p.Spec.CloudFormation.Stacks.Root
	})
}

type worker struct {
	ArchivedFiles       []provisioner.RemoteFileSpec
	CfnInitConfigSets   map[string]interface{}
	Files               []model.CustomFile
	SystemdUnits        []model.CustomSystemdUnit
	IAMPolicyStatements []model.IAMPolicyStatement
	NodeLabels          model.NodeLabels
	FeatureGates        model.FeatureGates
	Kubeconfig          string
	KubeletVolumeMounts []pluginmodel.VolumeMount
}

type controller struct {
	ArchivedFiles       []provisioner.RemoteFileSpec
	APIServerFlags      pluginmodel.APIServerFlags
	APIServerVolumes    pluginmodel.APIServerVolumes
	CfnInitConfigSets   map[string]interface{}
	Files               []model.CustomFile
	SystemdUnits        []model.CustomSystemdUnit
	IAMPolicyStatements []model.IAMPolicyStatement
	NodeLabels          model.NodeLabels
	Kubeconfig          string
	KubeletVolumeMounts []pluginmodel.VolumeMount
}

type etcd struct {
	Files               []model.CustomFile
	SystemdUnits        []model.CustomSystemdUnit
	IAMPolicyStatements []model.IAMPolicyStatement
}

func (e ClusterExtension) foreachEnabledPlugins(do func(p *pluginmodel.Plugin, pc *model.PluginConfig) error) error {
	for _, p := range e.plugins {
		if enabled, pc := p.EnabledIn(e.configs); enabled {
			if err := do(p, pc); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e ClusterExtension) stackExt(name string, src func(p *pluginmodel.Plugin) pluginmodel.Stack) (*stack, error) {
	resources := map[string]interface{}{}

	err := e.foreachEnabledPlugins(func(p *pluginmodel.Plugin, pc *model.PluginConfig) error {
		values := pluginutil.MergeValues(p.Spec.Values, pc.Values)

		render := plugincontents.NewTemplateRenderer(p, values, e.config)

		m, err := render.MapFromJsonContents(src(p).Resources.RemoteFileSpec)
		if err != nil {
			return fmt.Errorf("failed to load additional resources for %s stack: %v", name, err)
		}
		for k, v := range m {
			resources[k] = v
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &stack{
		Resources: resources,
	}, nil
}

func (e ClusterExtension) NodePoolStack() (*stack, error) {
	return e.stackExt("node-pool", func(p *pluginmodel.Plugin) pluginmodel.Stack {
		return p.Spec.CloudFormation.Stacks.NodePool
	})
}

func regularOrConfigSetFile(f provisioner.RemoteFileSpec, render *plugincontents.TemplateRenderer) (*string, map[string]interface{}, error) {
	goRendered, err := render.File(f)
	if err != nil {
		return nil, nil, err
	}

	tokens := tmpl.TextToCfnExprTokens(goRendered)

	if len(tokens) == 1 {
		return &tokens[0], nil, nil
	}

	return nil, map[string]interface{}{"Fn::Join": []interface{}{"", tokens}}, nil
}

func (e ClusterExtension) Worker(config interface{}) (*worker, error) {
	files := []model.CustomFile{}
	systemdUnits := []model.CustomSystemdUnit{}
	iamStatements := []model.IAMPolicyStatement{}
	nodeLabels := model.NodeLabels{}
	featureGates := model.FeatureGates{}
	configsets := map[string]interface{}{}
	archivedFiles := []provisioner.RemoteFileSpec{}
	var kubeconfig string
	kubeletMounts := []pluginmodel.VolumeMount{}

	for _, p := range e.plugins {
		if enabled, pc := p.EnabledIn(e.configs); enabled {
			values := pluginutil.MergeValues(p.Spec.Values, pc.Values)
			render := plugincontents.NewTemplateRenderer(p, values, config)

			for _, d := range p.Spec.Machine.Roles.Worker.Systemd.Units {
				u := model.CustomSystemdUnit{
					Name:    d.Name,
					Command: "start",
					Content: d.Contents.Content.String(),
					Enable:  true,
					Runtime: false,
				}
				systemdUnits = append(systemdUnits, u)
			}

			configsetFiles := map[string]interface{}{}
			for _, d := range p.Spec.Machine.Roles.Worker.Files {
				if d.IsBinary() {
					archivedFiles = append(archivedFiles, d)
					continue
				}

				s, cfg, err := regularOrConfigSetFile(d, render)
				if err != nil {
					return nil, fmt.Errorf("failed to load plugin worker file contents: %v", err)
				}

				var perm uint
				perm = d.Permissions

				if s != nil {
					f := model.CustomFile{
						Path:        d.Path,
						Permissions: perm,
						Content:     *s,
					}
					files = append(files, f)
				} else {
					configsetFiles[d.Path] = map[string]interface{}{
						"content": cfg,
					}
				}
			}
			configsets[p.Name] = map[string]interface{}{
				"files": configsetFiles,
			}

			iamStatements = append(iamStatements, p.Spec.Machine.Roles.Worker.IAM.Policy.Statements...)

			for k, v := range p.Spec.Machine.Roles.Worker.Kubelet.NodeLabels {
				nodeLabels[k] = v
			}

			for k, v := range p.Spec.Machine.Roles.Worker.Kubelet.FeatureGates {
				featureGates[k] = v
			}

			if p.Spec.Machine.Roles.Controller.Kubelet.Kubeconfig != "" {
				kubeconfig = p.Spec.Machine.Roles.Controller.Kubelet.Kubeconfig
			}

			if len(p.Spec.Machine.Roles.Controller.Kubelet.Mounts) > 0 {
				kubeletMounts = append(kubeletMounts, p.Spec.Machine.Roles.Controller.Kubelet.Mounts...)
			}
		}
	}

	return &worker{
		ArchivedFiles:       archivedFiles,
		CfnInitConfigSets:   configsets,
		Files:               files,
		SystemdUnits:        systemdUnits,
		IAMPolicyStatements: iamStatements,
		NodeLabels:          nodeLabels,
		FeatureGates:        featureGates,
		KubeletVolumeMounts: kubeletMounts,
		Kubeconfig:          kubeconfig,
	}, nil
}

func (e ClusterExtension) ControlPlaneStack() (*stack, error) {
	return e.stackExt("control-plane", func(p *pluginmodel.Plugin) pluginmodel.Stack {
		return p.Spec.CloudFormation.Stacks.ControlPlane
	})
}

func (e ClusterExtension) EtcdStack() (*stack, error) {
	return e.stackExt("etcd", func(p *pluginmodel.Plugin) pluginmodel.Stack {
		return p.Spec.CloudFormation.Stacks.Etcd
	})
}

func (e ClusterExtension) Controller(clusterConfig interface{}) (*controller, error) {
	apiServerFlags := pluginmodel.APIServerFlags{}
	apiServerVolumes := pluginmodel.APIServerVolumes{}
	systemdUnits := []model.CustomSystemdUnit{}
	files := []model.CustomFile{}
	iamStatements := model.IAMPolicyStatements{}
	nodeLabels := model.NodeLabels{}
	configsets := map[string]interface{}{}
	archivedFiles := []provisioner.RemoteFileSpec{}
	var kubeconfig string
	kubeletMounts := []pluginmodel.VolumeMount{}

	for _, p := range e.plugins {
		fmt.Fprintf(os.Stderr, "plugin=%+v configs=%+v", p, e.configs)
		if enabled, pc := p.EnabledIn(e.configs); enabled {
			values := pluginutil.MergeValues(p.Spec.Values, pc.Values)
			render := plugincontents.NewTemplateRenderer(p, values, clusterConfig)

			{
				for _, f := range p.Spec.Kubernetes.APIServer.Flags {
					v, err := render.String(f.Value)
					if err != nil {
						return nil, fmt.Errorf("failed to load apisersver flags: %v", err)
					}
					newFlag := pluginmodel.APIServerFlag{
						Name:  f.Name,
						Value: v,
					}
					apiServerFlags = append(apiServerFlags, newFlag)
				}
			}

			apiServerVolumes = append(apiServerVolumes, p.Spec.Kubernetes.APIServer.Volumes...)

			for _, d := range p.Spec.Machine.Roles.Controller.Systemd.Units {
				u := model.CustomSystemdUnit{
					Name:    d.Name,
					Command: "start",
					Content: d.Contents.Content.String(),
					Enable:  true,
					Runtime: false,
				}
				systemdUnits = append(systemdUnits, u)
			}

			for _, d := range p.Spec.Machine.Roles.Controller.Files {
				if d.IsBinary() {
					archivedFiles = append(archivedFiles, d)
					continue
				}

				dump, err := json.Marshal(d)
				if err != nil {
					panic(err)
				}
				fmt.Fprintf(os.Stderr, "controller file: %s", string(dump))

				s, cfg, err := regularOrConfigSetFile(d, render)
				if err != nil {
					return nil, fmt.Errorf("failed to load plugin controller file contents: %v", err)
				}

				if s != nil {
					fmt.Fprintf(os.Stderr, "controller file rendering result str: %s", *s)
				}

				if cfg != nil {
					fmt.Fprintf(os.Stderr, "controller file rendering result map: cfg=%+v", cfg)
				}

				perm := d.Permissions

				if s != nil {
					f := model.CustomFile{
						Path:        d.Path,
						Permissions: perm,
						Content:     *s,
					}
					files = append(files, f)
				} else {
					configsets[p.Name] = map[string]interface{}{
						"files": map[string]interface{}{
							d.Path: map[string]interface{}{
								"content": cfg,
							},
						},
					}
				}
			}

			iamStatements = append(iamStatements, p.Spec.Machine.Roles.Controller.IAM.Policy.Statements...)

			for k, v := range p.Spec.Machine.Roles.Controller.Kubelet.NodeLabels {
				nodeLabels[k] = v
			}

			if p.Spec.Machine.Roles.Controller.Kubelet.Kubeconfig != "" {
				kubeconfig = p.Spec.Machine.Roles.Controller.Kubelet.Kubeconfig
			}

			if len(p.Spec.Machine.Roles.Controller.Kubelet.Mounts) > 0 {
				kubeletMounts = append(kubeletMounts, p.Spec.Machine.Roles.Controller.Kubelet.Mounts...)
			}
		}
	}

	return &controller{
		ArchivedFiles:       archivedFiles,
		APIServerFlags:      apiServerFlags,
		APIServerVolumes:    apiServerVolumes,
		Files:               files,
		SystemdUnits:        systemdUnits,
		IAMPolicyStatements: iamStatements,
		NodeLabels:          nodeLabels,
		KubeletVolumeMounts: kubeletMounts,
		Kubeconfig:          kubeconfig,
	}, nil
}

func (e ClusterExtension) Etcd() (*etcd, error) {
	systemdUnits := []model.CustomSystemdUnit{}
	files := []model.CustomFile{}
	iamStatements := model.IAMPolicyStatements{}

	for _, p := range e.plugins {
		if enabled, _ := p.EnabledIn(e.configs); enabled {
			load := plugincontents.NewPluginFileLoader(p)

			for _, d := range p.Spec.Machine.Roles.Etcd.Systemd.Units {
				u := model.CustomSystemdUnit{
					Name:    d.Name,
					Command: "start",
					Content: d.Contents.Content.String(),
					Enable:  true,
					Runtime: false,
				}
				systemdUnits = append(systemdUnits, u)
			}

			for _, d := range p.Spec.Machine.Roles.Etcd.Files {
				s, err := load.String(d)
				if err != nil {
					return nil, fmt.Errorf("failed to load plugin etcd file contents: %v", err)
				}
				perm := d.Permissions
				f := model.CustomFile{
					Path:        d.Path,
					Permissions: perm,
					Content:     s,
				}
				files = append(files, f)
			}

			iamStatements = append(iamStatements, p.Spec.Machine.Roles.Etcd.IAM.Policy.Statements...)
		}
	}

	return &etcd{
		Files:               files,
		SystemdUnits:        systemdUnits,
		IAMPolicyStatements: iamStatements,
	}, nil
}
