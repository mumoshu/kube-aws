package clusterapi

type Autoscaling struct {
	ClusterAutoscaler ClusterAutoscaler `yaml:"clusterAutoscaler,omitempty"`
}
