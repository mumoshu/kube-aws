package clusterapi

type Networking struct {
	AmazonVPC   AmazonVPC   `yaml:"amazonVPC"`
	SelfHosting SelfHosting `yaml:"selfHosting"`
}

type SelfHosting struct {
	Type            string `yaml:"type"`
	Typha           bool   `yaml:"typha"`
	CalicoNodeImage Image  `yaml:"calicoNodeImage"`
	CalicoCniImage  Image  `yaml:"calicoCniImage"`
	FlannelImage    Image  `yaml:"flannelImage"`
	FlannelCniImage Image  `yaml:"flannelCniImage"`
	TyphaImage      Image  `yaml:"typhaImage"`
}
