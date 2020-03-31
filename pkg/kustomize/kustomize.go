package kustomize

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/rancher/fleet/pkg/content"
	"github.com/rancher/fleet/pkg/manifest"
	"github.com/rancher/wrangler/pkg/data/convert"
	"github.com/rancher/wrangler/pkg/slice"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/api/filesys"
	"sigs.k8s.io/kustomize/api/konfig"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	KustomizeYAML = "kustomization.yaml"
	ManifestsYAML = "manifests.yaml"
)

func Process(m *manifest.Manifest, content []byte, dir string) ([]runtime.Object, bool, error) {
	if dir == "" {
		dir = "."
	}

	fs, err := toFilesystem(m, dir, content)
	if err != nil {
		return nil, false, err
	}

	d := filepath.Join(dir, KustomizeYAML)
	if !fs.Exists(d) {
		return nil, false, nil
	}

	if len(content) > 0 {
		if err := modifyKustomize(fs, dir); err != nil {
			return nil, false, err
		}
	}

	objs, err := kustomize(fs, dir)
	return objs, true, err
}

func modifyKustomize(f filesys.FileSystem, dir string) error {
	file := filepath.Join(dir, KustomizeYAML)
	fileBytes, err := f.ReadFile(file)
	if err != nil {
		return err
	}

	data := map[string]interface{}{}
	if err := yaml.Unmarshal(fileBytes, &data); err != nil {
		return nil
	}

	resources := convert.ToStringSlice(data["resources"])
	if slice.ContainsString(resources, ManifestsYAML) {
		return nil
	}

	data["resources"] = append(resources, ManifestsYAML)
	fileBytes, err = json.Marshal(data)
	if err != nil {
		return err
	}

	return f.WriteFile(file, fileBytes)
}

func toFilesystem(m *manifest.Manifest, dir string, manifestsContent []byte) (filesys.FileSystem, error) {
	f := filesys.MakeEmptyDirInMemory()
	for _, resource := range m.Resources {
		if !strings.HasPrefix(resource.Name, "kustomize/") {
			continue
		}
		name := strings.TrimPrefix(resource.Name, "kustomize/")
		data, err := content.Decode(resource.Content, resource.Encoding)
		if err != nil {
			return nil, err
		}
		if _, err := f.AddFile(name, data); err != nil {
			return nil, err
		}
	}

	_, err := f.AddFile(filepath.Join(dir, ManifestsYAML), manifestsContent)
	return f, err
}

func kustomize(fs filesys.FileSystem, dir string) (result []runtime.Object, err error) {
	pcfg := konfig.DisabledPluginConfig()
	kust := krusty.MakeKustomizer(fs, &krusty.Options{
		LoadRestrictions: types.LoadRestrictionsRootOnly,
		PluginConfig:     pcfg,
	})
	resMap, err := kust.Run(dir)
	if err != nil {
		return nil, err
	}
	for _, m := range resMap.Resources() {
		result = append(result, &unstructured.Unstructured{
			Object: m.Map(),
		})
	}
	return
}