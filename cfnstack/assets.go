package cfnstack

import (
	"fmt"
	"github.com/kubernetes-incubator/kube-aws/fingerprint"
	"github.com/kubernetes-incubator/kube-aws/pkg/clusterapi"
	"path/filepath"
	"strings"
)

type Assets interface {
	Merge(Assets) Assets
	AsMap() map[clusterapi.AssetID]clusterapi.Asset
	FindAssetByStackAndFileName(string, string) (clusterapi.Asset, error)
}

func EmptyAssets() assetsImpl {
	return assetsImpl{
		underlying: map[clusterapi.AssetID]clusterapi.Asset{},
	}
}

type assetsImpl struct {
	underlying map[clusterapi.AssetID]clusterapi.Asset
}

func (a assetsImpl) Merge(other Assets) Assets {
	merged := map[clusterapi.AssetID]clusterapi.Asset{}

	for k, v := range a.underlying {
		merged[k] = v
	}
	for k, v := range other.AsMap() {
		merged[k] = v
	}

	return assetsImpl{
		underlying: merged,
	}
}

func (a assetsImpl) AsMap() map[clusterapi.AssetID]clusterapi.Asset {
	return a.underlying
}

func (a assetsImpl) findAssetByID(id clusterapi.AssetID) (clusterapi.Asset, error) {
	asset, ok := a.underlying[id]
	if !ok {
		return asset, fmt.Errorf("[bug] failed to get the asset for the id \"%s\"", id)
	}
	return asset, nil
}

func (a assetsImpl) FindAssetByStackAndFileName(stack string, file string) (clusterapi.Asset, error) {
	return a.findAssetByID(clusterapi.NewAssetID(stack, file))
}

type AssetsBuilder interface {
	Add(filename string, content string) (clusterapi.Asset, error)
	AddUserDataPart(userdata clusterapi.UserData, part string, assetName string) error
	Build() Assets
}

type assetsBuilderImpl struct {
	locProvider AssetLocationProvider
	assets      map[clusterapi.AssetID]clusterapi.Asset
}

func (b *assetsBuilderImpl) Add(filename string, content string) (clusterapi.Asset, error) {
	loc, err := b.locProvider.locationFor(filename)
	if err != nil {
		return clusterapi.Asset{}, err
	}

	asset := clusterapi.Asset{
		AssetLocation: *loc,
		Content:       content,
	}

	b.assets[loc.ID] = asset
	return asset, nil
}

func (b *assetsBuilderImpl) AddUserDataPart(userdata clusterapi.UserData, part string, assetName string) error {
	if p, ok := userdata.Parts[part]; ok {
		content, err := p.Template()
		if err != nil {
			return err
		}

		filename := fmt.Sprintf("%s-%s", assetName, fingerprint.SHA256(content))
		asset, err := b.Add(filename, content)
		if err != nil {
			return err
		}
		p.Asset = asset
	}
	return nil // it is not an error if part is not found
}

func (b *assetsBuilderImpl) Build() Assets {
	return assetsImpl{
		underlying: b.assets,
	}
}

func NewAssetsBuilder(stackName string, s3URI string, region clusterapi.Region) AssetsBuilder {
	return &assetsBuilderImpl{
		locProvider: AssetLocationProvider{
			s3URI:     s3URI,
			region:    region,
			stackName: stackName,
		},
		assets: map[clusterapi.AssetID]clusterapi.Asset{},
	}
}

type AssetLocationProvider struct {
	s3URI     string
	region    clusterapi.Region
	stackName string
}

func (p AssetLocationProvider) locationFor(filename string) (*clusterapi.AssetLocation, error) {
	if filename == "" {
		return nil, fmt.Errorf("Can't produce S3 location for empty filename")
	}
	s3URI := p.s3URI

	uri, err := S3URIFromString(s3URI)

	if err != nil {
		return nil, fmt.Errorf("failed to determine location for %s: %v", filename, err)
	}

	relativePathComponents := []string{
		p.stackName,
		filename,
	}

	key := strings.Join(
		append(uri.PathComponents(), relativePathComponents...),
		"/",
	)

	id := clusterapi.NewAssetID(p.stackName, filename)

	return &clusterapi.AssetLocation{
		ID:     id,
		Key:    key,
		Bucket: uri.Bucket(),
		Path:   filepath.Join(relativePathComponents...),
		Region: p.region,
	}, nil
}
