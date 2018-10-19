package clusterextension

import (
	"fmt"

	"encoding/json"
	"github.com/kubernetes-incubator/kube-aws/pkg/clusterapi"
	"github.com/kubernetes-incubator/kube-aws/plugin/plugincontents"
	"github.com/kubernetes-incubator/kube-aws/plugin/pluginutil"
	"github.com/kubernetes-incubator/kube-aws/provisioner"
	"github.com/kubernetes-incubator/kube-aws/tmpl"
	//"os"
	"path/filepath"
)

type ClusterExtension struct {
	plugins []*clusterapi.Plugin
	configs clusterapi.PluginConfigs
}

func NewExtrasFromPlugins(plugins []*clusterapi.Plugin, configs clusterapi.PluginConfigs) ClusterExtension {
	return ClusterExtension{
		plugins: plugins,
		configs: configs,
	}
}

func NewExtras() ClusterExtension {
	return ClusterExtension{
		plugins: []*clusterapi.Plugin{},
		configs: clusterapi.PluginConfigs{},
	}
}

type stack struct {
	Resources map[string]interface{}
}

func (e ClusterExtension) KeyPairSpecs() []clusterapi.KeyPairSpec {
	keypairs := []clusterapi.KeyPairSpec{}
	err := e.foreachEnabledPlugins(func(p *clusterapi.Plugin, pc *clusterapi.PluginConfig) error {
		for _, spec := range p.Spec.Cluster.PKI.KeyPairs {
			keypairs = append(keypairs, spec)
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	return keypairs
}

func (e ClusterExtension) RootStack(config interface{}) (*stack, error) {
	return e.stackExt("root", config, func(p *clusterapi.Plugin) clusterapi.Stack {
		return p.Spec.Cluster.CloudFormation.Stacks.Root
	})
}

type worker struct {
	ArchivedFiles       []provisioner.RemoteFileSpec
	CfnInitConfigSets   map[string]interface{}
	Files               []clusterapi.CustomFile
	SystemdUnits        []clusterapi.CustomSystemdUnit
	IAMPolicyStatements []clusterapi.IAMPolicyStatement
	NodeLabels          clusterapi.NodeLabels
	FeatureGates        clusterapi.FeatureGates
	Kubeconfig          string
	KubeletVolumeMounts []clusterapi.ContainerVolumeMount
}

type controller struct {
	ArchivedFiles       []provisioner.RemoteFileSpec
	APIServerFlags      clusterapi.APIServerFlags
	APIServerVolumes    clusterapi.APIServerVolumes
	CfnInitConfigSets   map[string]interface{}
	Files               []clusterapi.CustomFile
	SystemdUnits        []clusterapi.CustomSystemdUnit
	IAMPolicyStatements []clusterapi.IAMPolicyStatement
	NodeLabels          clusterapi.NodeLabels
	Kubeconfig          string
	KubeletVolumeMounts []clusterapi.ContainerVolumeMount

	KubernetesManifestFiles []*provisioner.RemoteFile
	HelmReleaseFilesets     []clusterapi.HelmReleaseFileset
}

type etcd struct {
	Files               []clusterapi.CustomFile
	SystemdUnits        []clusterapi.CustomSystemdUnit
	IAMPolicyStatements []clusterapi.IAMPolicyStatement
}

func (e ClusterExtension) foreachEnabledPlugins(do func(p *clusterapi.Plugin, pc *clusterapi.PluginConfig) error) error {
	for _, p := range e.plugins {
		if enabled, pc := p.EnabledIn(e.configs); enabled {
			if err := do(p, pc); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e ClusterExtension) stackExt(name string, config interface{}, src func(p *clusterapi.Plugin) clusterapi.Stack) (*stack, error) {
	resources := map[string]interface{}{}

	err := e.foreachEnabledPlugins(func(p *clusterapi.Plugin, pc *clusterapi.PluginConfig) error {
		values := pluginutil.MergeValues(p.Spec.Cluster.Values, pc.Values)

		render := plugincontents.NewTemplateRenderer(p, values, config)

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

func (e ClusterExtension) NodePoolStack(config interface{}) (*stack, error) {
	return e.stackExt("node-pool", config, func(p *clusterapi.Plugin) clusterapi.Stack {
		return p.Spec.Cluster.CloudFormation.Stacks.NodePool
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
	files := []clusterapi.CustomFile{}
	systemdUnits := []clusterapi.CustomSystemdUnit{}
	iamStatements := []clusterapi.IAMPolicyStatement{}
	nodeLabels := clusterapi.NodeLabels{}
	featureGates := clusterapi.FeatureGates{}
	configsets := map[string]interface{}{}
	archivedFiles := []provisioner.RemoteFileSpec{}
	var kubeconfig string
	kubeletMounts := []clusterapi.ContainerVolumeMount{}

	for _, p := range e.plugins {
		if enabled, pc := p.EnabledIn(e.configs); enabled {
			values := pluginutil.MergeValues(p.Spec.Cluster.Values, pc.Values)
			render := plugincontents.NewTemplateRenderer(p, values, config)

			for _, d := range p.Spec.Cluster.Machine.Roles.Worker.Systemd.Units {
				u := clusterapi.CustomSystemdUnit{
					Name:    d.Name,
					Command: "start",
					Content: d.Contents.Content.String(),
					Enable:  true,
					Runtime: false,
				}
				systemdUnits = append(systemdUnits, u)
			}

			configsetFiles := map[string]interface{}{}
			for _, d := range p.Spec.Cluster.Machine.Roles.Worker.Files {
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
					f := clusterapi.CustomFile{
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

			iamStatements = append(iamStatements, p.Spec.Cluster.Machine.Roles.Worker.IAM.Policy.Statements...)

			for k, v := range p.Spec.Cluster.Machine.Roles.Worker.Kubelet.NodeLabels {
				nodeLabels[k] = v
			}

			for k, v := range p.Spec.Cluster.Machine.Roles.Worker.Kubelet.FeatureGates {
				featureGates[k] = v
			}

			if p.Spec.Cluster.Machine.Roles.Controller.Kubelet.Kubeconfig != "" {
				kubeconfig = p.Spec.Cluster.Machine.Roles.Controller.Kubelet.Kubeconfig
			}

			if len(p.Spec.Cluster.Machine.Roles.Controller.Kubelet.Mounts) > 0 {
				kubeletMounts = append(kubeletMounts, p.Spec.Cluster.Machine.Roles.Controller.Kubelet.Mounts...)
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

func (e ClusterExtension) ControlPlaneStack(config interface{}) (*stack, error) {
	return e.stackExt("control-plane", config, func(p *clusterapi.Plugin) clusterapi.Stack {
		return p.Spec.Cluster.CloudFormation.Stacks.ControlPlane
	})
}

func (e ClusterExtension) EtcdStack(config interface{}) (*stack, error) {
	return e.stackExt("etcd", config, func(p *clusterapi.Plugin) clusterapi.Stack {
		return p.Spec.Cluster.CloudFormation.Stacks.Etcd
	})
}

func (e ClusterExtension) Controller(clusterConfig interface{}) (*controller, error) {
	apiServerFlags := clusterapi.APIServerFlags{}
	apiServerVolumes := clusterapi.APIServerVolumes{}
	systemdUnits := []clusterapi.CustomSystemdUnit{}
	files := []clusterapi.CustomFile{}
	iamStatements := clusterapi.IAMPolicyStatements{}
	nodeLabels := clusterapi.NodeLabels{}
	configsets := map[string]interface{}{}
	archivedFiles := []provisioner.RemoteFileSpec{}
	var kubeconfig string
	kubeletMounts := []clusterapi.ContainerVolumeMount{}
	manifests := []*provisioner.RemoteFile{}
	releaseFilesets := []clusterapi.HelmReleaseFileset{}

	for _, p := range e.plugins {
		//fmt.Fprintf(os.Stderr, "plugin=%+v configs=%+v", p, e.configs)
		if enabled, pc := p.EnabledIn(e.configs); enabled {
			values := pluginutil.MergeValues(p.Spec.Cluster.Values, pc.Values)
			render := plugincontents.NewTemplateRenderer(p, values, clusterConfig)

			{
				for _, f := range p.Spec.Cluster.Kubernetes.APIServer.Flags {
					v, err := render.String(f.Value)
					if err != nil {
						return nil, fmt.Errorf("failed to load apisersver flags: %v", err)
					}
					newFlag := clusterapi.APIServerFlag{
						Name:  f.Name,
						Value: v,
					}
					apiServerFlags = append(apiServerFlags, newFlag)
				}
			}

			apiServerVolumes = append(apiServerVolumes, p.Spec.Cluster.Kubernetes.APIServer.Volumes...)

			for _, d := range p.Spec.Cluster.Machine.Roles.Controller.Systemd.Units {
				u := clusterapi.CustomSystemdUnit{
					Name:    d.Name,
					Command: "start",
					Content: d.Contents.Content.String(),
					Enable:  true,
					Runtime: false,
				}
				systemdUnits = append(systemdUnits, u)
			}

			for _, d := range p.Spec.Cluster.Machine.Roles.Controller.Files {
				if d.IsBinary() {
					archivedFiles = append(archivedFiles, d)
					continue
				}

				//dump, err := json.Marshal(d)
				//if err != nil {
				//	panic(err)
				//}
				//fmt.Fprintf(os.Stderr, "controller file: %s", string(dump))

				s, cfg, err := regularOrConfigSetFile(d, render)
				if err != nil {
					return nil, fmt.Errorf("failed to load plugin controller file contents: %v", err)
				}

				//if s != nil {
				//	fmt.Fprintf(os.Stderr, "controller file rendering result str: %s", *s)
				//}
				//
				//if cfg != nil {
				//	fmt.Fprintf(os.Stderr, "controller file rendering result map: cfg=%+v", cfg)
				//}

				perm := d.Permissions

				if s != nil {
					var path string
					if d.Type == "credential" {
						path = d.Path + ".enc"
					} else {
						path = d.Path
					}
					f := clusterapi.CustomFile{
						Path:        path,
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

			iamStatements = append(iamStatements, p.Spec.Cluster.Machine.Roles.Controller.IAM.Policy.Statements...)

			for k, v := range p.Spec.Cluster.Machine.Roles.Controller.Kubelet.NodeLabels {
				nodeLabels[k] = v
			}

			if p.Spec.Cluster.Machine.Roles.Controller.Kubelet.Kubeconfig != "" {
				kubeconfig = p.Spec.Cluster.Machine.Roles.Controller.Kubelet.Kubeconfig
			}

			if len(p.Spec.Cluster.Machine.Roles.Controller.Kubelet.Mounts) > 0 {
				kubeletMounts = append(kubeletMounts, p.Spec.Cluster.Machine.Roles.Controller.Kubelet.Mounts...)
			}

			for _, m := range p.Spec.Cluster.Kubernetes.Manifests {
				rendered, err := render.File(m.RemoteFileSpec)
				if err != nil {
					panic(err)
				}
				var name string
				if m.Name == "" {
					if m.RemoteFileSpec.Source.Path == "" {
						panic(fmt.Errorf("manifest.name is required in %v", m))
					}
					name = filepath.Base(m.RemoteFileSpec.Source.Path)
				} else {
					name = m.Name
				}
				f := provisioner.NewRemoteFileAtPath(
					filepath.Join("/srv/kube-aws/plugins", p.Metadata.Name, name),
					[]byte(rendered),
				)

				manifests = append(manifests, f)
			}

			for _, releaseConfig := range p.Spec.Cluster.Helm.Releases {
				valuesFilePath := filepath.Join("/srv/kube-aws/plugins", p.Metadata.Name, "helm", "releases", releaseConfig.Name, "values.yaml")
				valuesFileContent, err := json.Marshal(releaseConfig.Values)
				if err != nil {
					panic(fmt.Errorf("Unexpected error in HelmReleasePlugin: %v", err))
				}
				releaseFileData := map[string]interface{}{
					"values": map[string]string{
						"file": valuesFilePath,
					},
					"chart": map[string]string{
						"name":    releaseConfig.Name,
						"version": releaseConfig.Version,
					},
				}
				releaseFilePath := filepath.Join("/srv/kube-aws/plugins", p.Metadata.Name, "helm", "releases", releaseConfig.Name, "release.json")
				releaseFileContent, err := json.Marshal(releaseFileData)
				if err != nil {
					panic(fmt.Errorf("Unexpected error in HelmReleasePlugin: %v", err))
				}
				r := clusterapi.HelmReleaseFileset{
					ValuesFile: provisioner.NewRemoteFileAtPath(
						valuesFilePath,
						valuesFileContent,
					),
					ReleaseFile: provisioner.NewRemoteFileAtPath(
						releaseFilePath,
						releaseFileContent,
					),
				}
				releaseFilesets = append(releaseFilesets, r)
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

		KubernetesManifestFiles: manifests,
		HelmReleaseFilesets:     releaseFilesets,
	}, nil
}

func (e ClusterExtension) Etcd() (*etcd, error) {
	systemdUnits := []clusterapi.CustomSystemdUnit{}
	files := []clusterapi.CustomFile{}
	iamStatements := clusterapi.IAMPolicyStatements{}

	for _, p := range e.plugins {
		if enabled, _ := p.EnabledIn(e.configs); enabled {
			load := plugincontents.NewPluginFileLoader(p)

			for _, d := range p.Spec.Cluster.Machine.Roles.Etcd.Systemd.Units {
				u := clusterapi.CustomSystemdUnit{
					Name:    d.Name,
					Command: "start",
					Content: d.Contents.Content.String(),
					Enable:  true,
					Runtime: false,
				}
				systemdUnits = append(systemdUnits, u)
			}

			for _, d := range p.Spec.Cluster.Machine.Roles.Etcd.Files {
				s, err := load.String(d)
				if err != nil {
					return nil, fmt.Errorf("failed to load plugin etcd file contents: %v", err)
				}
				perm := d.Permissions
				f := clusterapi.CustomFile{
					Path:        d.Path,
					Permissions: perm,
					Content:     s,
				}
				files = append(files, f)
			}

			iamStatements = append(iamStatements, p.Spec.Cluster.Machine.Roles.Etcd.IAM.Policy.Statements...)
		}
	}

	return &etcd{
		Files:               files,
		SystemdUnits:        systemdUnits,
		IAMPolicyStatements: iamStatements,
	}, nil
}
