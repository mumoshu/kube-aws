package config

type Kubernetes struct {
	Authentication    KubernetesAuthentication `yaml:"authentication"`
	EncryptionAtRest  EncryptionAtRest         `yaml:"encryptionAtRest"`
	Networking        Networking               `yaml:"networking,omitempty"`
	ControllerManager ControllerManager        `yaml:"controllerManager,omitempty"`
}

type ControllerManager struct {
	ComputeResources ComputeResources `yaml:"resources,omitempty"`
}

type ComputeResources struct {
	Requests ResourceQuota `yaml:"requests,omitempty"`
	Limits   ResourceQuota `yaml:"limits,omitempty"`
}

type ResourceQuota struct {
	Cpu    string `yaml:"cpu"`
	Memory string `yaml:"memory"`
}
