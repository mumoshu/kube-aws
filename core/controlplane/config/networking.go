package config

import "github.com/kubernetes-incubator/kube-aws/model"

type Networking struct {
	AmazonVPC   AmazonVPC   `yaml:"amazonVPC"`
	SelfHosting SelfHosting `yaml:"selfHosting"`
}

type SelfHosting struct {
	Type            string      `yaml:"type"`
	Typha           bool        `yaml:"typha"`
	CalicoNodeImage model.Image `yaml:"calicoNodeImage"`
	CalicoCniImage  model.Image `yaml:"calicoCniImage"`
	FlannelImage    model.Image `yaml:"flannelImage"`
	FlannelCniImage model.Image `yaml:"flannelCniImage"`
	TyphaImage      model.Image `yaml:"typhaImage"`
}
