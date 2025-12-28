package ghpm

import "runtime/debug"

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
	BuiltBy = "unknown"
)

type BuildInfoData struct {
	Version string
	Commit  string
	Date    string
	BuiltBy string
}

func BuildInfo() BuildInfoData {
	info := BuildInfoData{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
		BuiltBy: BuiltBy,
	}
	fillFromRuntime(&info)
	if info.Version == "" {
		info.Version = "dev"
	}
	return info
}

func fillFromRuntime(info *BuildInfoData) {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if (info.Version == "" || info.Version == "dev") && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		info.Version = bi.Main.Version
	}
	for _, setting := range bi.Settings {
		switch setting.Key {
		case "vcs.revision":
			if info.Commit == "" || info.Commit == "none" {
				info.Commit = setting.Value
			}
		case "vcs.time":
			if info.Date == "" || info.Date == "unknown" {
				info.Date = setting.Value
			}
		}
	}
}
