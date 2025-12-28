package manifest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Manifest struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Source      Source   `yaml:"source"`
	Install     []Action `yaml:"install"`
	PostInstall []string `yaml:"postInstall"`
	PostRemove  []string `yaml:"postRemove"`
	Path        string   `yaml:"-"`
}

type Source struct {
	Kind string `yaml:"kind"`
	Repo string `yaml:"repo"`
}

type Action struct {
	Type    string
	Asset   *AssetAction
	URL     *URLAction
	File    *FileAction
	Symlink *SymlinkAction
	Extract *ExtractAction
	Mkdir   *MkdirAction
	Raw     map[string]any
}

type AssetAction struct {
	Name     string `yaml:"name"`
	Pattern  string `yaml:"pattern"`
	Target   string `yaml:"target"`
	Mode     string `yaml:"mode"`
	Preserve bool   `yaml:"preserve"`
}

type URLAction struct {
	URL      string `yaml:"url"`
	Target   string `yaml:"target"`
	Mode     string `yaml:"mode"`
	Preserve bool   `yaml:"preserve"`
}

type FileAction struct {
	Path     string `yaml:"path"`
	Target   string `yaml:"target"`
	Mode     string `yaml:"mode"`
	Preserve bool   `yaml:"preserve"`
}

type SymlinkAction struct {
	Target string `yaml:"target"`
	To     string `yaml:"to"`
}

type ExtractAction struct {
	From            ExtractFrom `yaml:"from"`
	Format          string      `yaml:"format"`
	StripComponents int         `yaml:"stripComponents"`
	TargetDir       string      `yaml:"targetDir"`
	Pick            []string    `yaml:"pick"`
	Omit            []string    `yaml:"omit"`
}

type ExtractFrom struct {
	Type    string `yaml:"type"`
	Name    string `yaml:"name"`
	Pattern string `yaml:"pattern"`
	URL     string `yaml:"url"`
	Path    string `yaml:"path"`
}

type MkdirAction struct {
	Path  string `yaml:"path"`
	Mode  string `yaml:"mode"`
	Owner string `yaml:"owner"`
	Group string `yaml:"group"`
}

func Load(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("%s: %w", path, err)
	}
	m.Path = path
	if m.Name == "" {
		m.Name = filepath.Base(filepath.Dir(path))
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

func (m Manifest) PackageDir() string {
	return filepath.Dir(m.Path)
}

func (m Manifest) Validate() error {
	if m.Name == "" {
		return errors.New("manifest name is required")
	}
	for i, action := range m.Install {
		if action.Type == "" {
			return fmt.Errorf("install[%d].type is required", i)
		}
		switch action.Type {
		case "asset":
			if action.Asset == nil {
				return fmt.Errorf("install[%d].asset is required", i)
			}
			if action.Asset.Name == "" && action.Asset.Pattern == "" {
				return fmt.Errorf("install[%d].asset.name or pattern is required", i)
			}
			if action.Asset.Target == "" {
				return fmt.Errorf("install[%d].asset.target is required", i)
			}
		case "url":
			if action.URL == nil {
				return fmt.Errorf("install[%d].url is required", i)
			}
			if action.URL.URL == "" {
				return fmt.Errorf("install[%d].url.url is required", i)
			}
			if action.URL.Target == "" {
				return fmt.Errorf("install[%d].url.target is required", i)
			}
		case "file":
			if action.File == nil {
				return fmt.Errorf("install[%d].file is required", i)
			}
			if action.File.Path == "" {
				return fmt.Errorf("install[%d].file.path is required", i)
			}
			if action.File.Target == "" {
				return fmt.Errorf("install[%d].file.target is required", i)
			}
		case "symlink":
			if action.Symlink == nil {
				return fmt.Errorf("install[%d].symlink is required", i)
			}
			if action.Symlink.Target == "" || action.Symlink.To == "" {
				return fmt.Errorf("install[%d].symlink.target and to are required", i)
			}
		case "extract":
			if action.Extract == nil {
				return fmt.Errorf("install[%d].extract is required", i)
			}
			if action.Extract.From.Type == "" {
				return fmt.Errorf("install[%d].extract.from.type is required", i)
			}
			switch action.Extract.From.Type {
			case "asset":
				if action.Extract.From.Name == "" && action.Extract.From.Pattern == "" {
					return fmt.Errorf("install[%d].extract.from.name or pattern is required", i)
				}
			case "url":
				if action.Extract.From.URL == "" {
					return fmt.Errorf("install[%d].extract.from.url is required", i)
				}
			case "file":
				if action.Extract.From.Path == "" {
					return fmt.Errorf("install[%d].extract.from.path is required", i)
				}
			default:
				return fmt.Errorf("install[%d].extract.from.type %q is unsupported", i, action.Extract.From.Type)
			}
			if action.Extract.TargetDir == "" {
				return fmt.Errorf("install[%d].extract.targetDir is required", i)
			}
		case "mkdir":
			if action.Mkdir == nil {
				return fmt.Errorf("install[%d].mkdir is required", i)
			}
			if action.Mkdir.Path == "" {
				return fmt.Errorf("install[%d].mkdir.path is required", i)
			}
		}
	}
	return nil
}

func (a *Action) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("action must be a mapping (line %d)", value.Line)
	}
	raw := map[string]any{}
	if err := value.Decode(&raw); err != nil {
		return fmt.Errorf("invalid action (line %d): %w", value.Line, err)
	}
	typ, _ := raw["type"].(string)
	if typ == "" {
		return fmt.Errorf("action.type is required (line %d)", value.Line)
	}
	a.Type = typ
	a.Raw = raw
	switch typ {
	case "asset":
		var v AssetAction
		if err := value.Decode(&v); err != nil {
			return fmt.Errorf("invalid asset action (line %d): %w", value.Line, err)
		}
		a.Asset = &v
	case "url":
		var v URLAction
		if err := value.Decode(&v); err != nil {
			return fmt.Errorf("invalid url action (line %d): %w", value.Line, err)
		}
		a.URL = &v
	case "file":
		var v FileAction
		if err := value.Decode(&v); err != nil {
			return fmt.Errorf("invalid file action (line %d): %w", value.Line, err)
		}
		a.File = &v
	case "symlink":
		var v SymlinkAction
		if err := value.Decode(&v); err != nil {
			return fmt.Errorf("invalid symlink action (line %d): %w", value.Line, err)
		}
		a.Symlink = &v
	case "extract":
		var v ExtractAction
		if err := value.Decode(&v); err != nil {
			return fmt.Errorf("invalid extract action (line %d): %w", value.Line, err)
		}
		a.Extract = &v
	case "mkdir":
		var v MkdirAction
		if err := value.Decode(&v); err != nil {
			return fmt.Errorf("invalid mkdir action (line %d): %w", value.Line, err)
		}
		a.Mkdir = &v
	default:
		return fmt.Errorf("unknown action type %q (line %d)", typ, value.Line)
	}
	return nil
}

type TemplateContext struct {
	Version string
	Tag     string
	OS      string
	Arch    string
	Repo    string
	Name    string
}

func ExpandTemplate(input string, ctx TemplateContext) string {
	replacer := strings.NewReplacer(
		"{version}", ctx.Version,
		"{tag}", ctx.Tag,
		"{os}", ctx.OS,
		"{arch}", ctx.Arch,
		"{repo}", ctx.Repo,
		"{name}", ctx.Name,
	)
	return replacer.Replace(input)
}

func MatchPattern(name, pattern string) bool {
	if pattern == "" {
		return false
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return strings.Contains(name, pattern)
	}
	return re.MatchString(name)
}
