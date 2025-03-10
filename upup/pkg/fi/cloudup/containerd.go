/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cloudup

import (
	"fmt"
	"net/url"

	"github.com/blang/semver/v4"

	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/assets"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/util/pkg/architectures"
	"k8s.io/kops/util/pkg/hashing"
)

const (
	// containerd packages URLs for v1.4.x+
	containerdVersionUrlAmd64 = "https://github.com/containerd/containerd/releases/download/v%s/cri-containerd-cni-%s-linux-amd64.tar.gz"
	// containerd legacy packages URLs for v1.2.x and v1.3.x
	containerdLegacyUrlAmd64 = "https://storage.googleapis.com/cri-containerd-release/cri-containerd-%s.linux-amd64.tar.gz"
	// containerd version that is available for both AMD64 and ARM64, used in case the selected version is not available for ARM64
	containerdFallbackVersion = "1.4.6"
)

func findContainerdAsset(c *kops.Cluster, assetBuilder *assets.AssetBuilder, arch architectures.Architecture) (*url.URL, *hashing.Hash, error) {
	if c.Spec.Containerd == nil {
		return nil, nil, fmt.Errorf("unable to find containerd config")
	}
	containerd := c.Spec.Containerd

	if containerd.Packages != nil {
		if arch == architectures.ArchitectureAmd64 && containerd.Packages.UrlAmd64 != nil && containerd.Packages.HashAmd64 != nil {
			assetUrl := fi.StringValue(containerd.Packages.UrlAmd64)
			assetHash := fi.StringValue(containerd.Packages.HashAmd64)
			return findAssetsUrlHash(assetBuilder, assetUrl, assetHash)
		}
		if arch == architectures.ArchitectureArm64 && containerd.Packages.UrlArm64 != nil && containerd.Packages.HashArm64 != nil {
			assetUrl := fi.StringValue(containerd.Packages.UrlArm64)
			assetHash := fi.StringValue(containerd.Packages.HashArm64)
			return findAssetsUrlHash(assetBuilder, assetUrl, assetHash)
		}
	}

	version := fi.StringValue(containerd.Version)
	if version == "" {
		return nil, nil, fmt.Errorf("unable to find containerd version")
	}
	assetUrl, assetHash, err := findContainerdVersionUrlHash(arch, version)
	if err != nil {
		return nil, nil, err
	}

	return findAssetsUrlHash(assetBuilder, assetUrl, assetHash)
}

func findContainerdVersionUrlHash(arch architectures.Architecture, version string) (u string, h string, e error) {
	var containerdAssetUrl, containerdAssetHash string

	if findAllContainerdHashesAmd64()[version] != "" {
		var err error
		containerdAssetUrl, err = findContainerdVersionUrl(arch, version)
		if err != nil {
			return "", "", err
		}
		containerdAssetHash, err = findContainerdVersionHash(arch, version)
		if err != nil {
			return "", "", err
		}
	} else {
		// Fall back to Docker packages
		dv := findAllContainerdDockerMappings()[version]
		if dv != "" {
			var err error
			containerdAssetUrl, err = findDockerVersionUrl(arch, dv)
			if err != nil {
				return "", "", err
			}
			containerdAssetHash, err = findDockerVersionHash(arch, dv)
			if err != nil {
				return "", "", err
			}
			println(dv)
		} else {
			return "", "", fmt.Errorf("unknown url and hash for containerd version: %s - %s", arch, version)
		}
	}

	return containerdAssetUrl, containerdAssetHash, nil
}

func findContainerdVersionUrl(arch architectures.Architecture, version string) (string, error) {
	sv, err := semver.ParseTolerant(version)
	if err != nil {
		return "", fmt.Errorf("unable to parse version string: %q", version)
	}
	if sv.LT(semver.MustParse("1.3.4")) {
		return "", fmt.Errorf("unsupported legacy containerd version: %q", version)
	}

	var u string
	switch arch {
	case architectures.ArchitectureAmd64:
		if sv.GTE(semver.MustParse("1.3.8")) {
			u = fmt.Sprintf(containerdVersionUrlAmd64, version, version)
		} else {
			u = fmt.Sprintf(containerdLegacyUrlAmd64, version)
		}
	case architectures.ArchitectureArm64:
		// For now there are only official AMD64 builds, always using fallback Docker version instead
		if findAllContainerdHashesAmd64()[version] != "" {
			if findAllContainerdDockerMappings()[version] != "" {
				u = fmt.Sprintf(dockerVersionUrlArm64, findAllContainerdDockerMappings()[version])
			} else {
				u = fmt.Sprintf(dockerVersionUrlArm64, findAllContainerdDockerMappings()[containerdFallbackVersion])
			}
		}
	default:
		return "", fmt.Errorf("unknown arch: %q", arch)
	}

	if u == "" {
		return "", fmt.Errorf("unknown url for containerd version: %s - %s", arch, version)
	}

	return u, nil
}

func findContainerdVersionHash(arch architectures.Architecture, version string) (string, error) {
	sv, err := semver.ParseTolerant(version)
	if err != nil {
		return "", fmt.Errorf("unable to parse version string: %q", version)
	}
	if sv.LT(semver.MustParse("1.3.4")) {
		return "", fmt.Errorf("unsupported legacy containerd version: %q", version)
	}

	var h string
	switch arch {
	case architectures.ArchitectureAmd64:
		h = findAllContainerdHashesAmd64()[version]
	case architectures.ArchitectureArm64:
		// For now there are only official AMD64 builds, always using fallback Docker version instead
		if findAllContainerdHashesAmd64()[version] != "" {
			if findAllContainerdDockerMappings()[version] != "" {
				h = findAllDockerHashesArm64()[findAllContainerdDockerMappings()[version]]
			} else {
				h = findAllDockerHashesArm64()[findAllContainerdDockerMappings()[containerdFallbackVersion]]
			}
		}
	default:
		return "", fmt.Errorf("unknown arch: %q", arch)
	}

	if h == "" {
		return "", fmt.Errorf("unknown hash for containerd version: %s - %s", arch, version)
	}

	return h, nil
}

func findAllContainerdHashesAmd64() map[string]string {
	hashes := map[string]string{
		"1.3.4":  "4616971c3ad21c24f2f2320fa1c085577a91032a068dd56a41c7c4b71a458087",
		"1.3.9":  "96663699e0f888fbf232ae6629a367aa7421f6b95044e7ee5d4d4e02841fac75",
		"1.3.10": "69e23e49cdf1232d475a77bf7ecd7145ff4a80295154e190125c4d8a20e241da",
		"1.4.0":  "b379f29417efd583f77e095173d4d0bd6bb001f0081b2a63d152ee7aef653ce1",
		"1.4.1":  "757efb93a4f3161efc447a943317503d8a7ded5cb4cc0cba3f3318d7ce1542ed",
		"1.4.2":  "9d0fd5f4d2bc58b345728432b7daac75fc99c1da91afa4f41e6103f618e74012",
		"1.4.3":  "2697a342e3477c211ab48313e259fd7e32ad1f5ded19320e6a559f50a82bff3d",
		"1.4.4":  "96641849cb78a0a119223a427dfdc1ade88412ef791a14193212c8c8e29d447b",
		"1.4.5":  "f8155278fd256526ca9804219e1ee46f5db11c6ddf455086b04c0887c868822a",
		"1.4.6":  "6ae4763598c9583f8b50605f19d6c7e9ef93c216706465e73dfc84ee6b63a238",
		"1.4.7":  "daa14638344fe0772f645e190e4d8eb9549b52743364cb8000521490f9e410b8",
		"1.4.8":  "96e815c9ab664a02dd5be35e31d15890ea6bef04dfaa39f99f14676c3d6561e8",
		"1.4.9":  "9911479f86012d6eab7e0f532da8f807a8b0f555ee09ef89367d8c31243073bb",
		"1.5.0":  "aee7b553ab88842fdafe43955757abe746b8e9995b2be55c603f0a236186ff9b",
		"1.5.1":  "2fd97916b24396c13849cfcd89805170e1ef0265a2f7fce8e74ae044a6a6a169",
		"1.5.2":  "e7adbb6c6f6e67639460579a8aa991e9ce4de2062ed36d3261e6e4865574d947",
		"1.5.3":  "32a9bf1b7ab2adbd9d2a16b17bf1aa6e61592938655adfb5114c40d527aa9be7",
		"1.5.4":  "591e4e087ea2f5007e6c64deb382df58d419b7b6922eab45a1923d843d57615f",
		"1.5.5":  "45f02cfc65db47cf088c95555906e1dcba7baf5a3fbad3d947dd6b9af476a144",
	}

	return hashes
}

func findAllContainerdDockerMappings() map[string]string {
	versions := map[string]string{
		"1.3.7": "19.03.13",
		"1.3.9": "19.03.14",
		"1.4.3": "20.10.0",
		"1.4.4": "20.10.6",
		"1.4.6": "20.10.7",
	}

	return versions
}
