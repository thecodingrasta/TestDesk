package prefs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ProjectRef struct {
	Root         string    `json:"root"`
	Name         string    `json:"name"`
	LastOpenedAt time.Time `json:"lastOpenedAt"`
}

type Preferences struct {
	LastProjectRoot string       `json:"lastProjectRoot,omitempty"`
	Projects        []ProjectRef `json:"projects,omitempty"`
	UpdatedAt       time.Time    `json:"updatedAt"`
}

func Path() (string, error) {
	if override := strings.TrimSpace(os.Getenv("TESTDESK_CONFIG_HOME")); override != "" {
		return filepath.Join(override, "TestDesk", "preferences.json"), nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "TestDesk", "preferences.json"), nil
}

func Load() (Preferences, error) {
	path, err := Path()
	if err != nil {
		return Preferences{}, err
	}
	data, err := readFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Preferences{}, nil
	}
	if err != nil {
		return Preferences{}, err
	}
	var value Preferences
	if err := json.Unmarshal(data, &value); err != nil {
		return Preferences{}, err
	}
	return normalise(value), nil
}

func Save(value Preferences) error {
	return withPreferencesLock(func() error { return saveUnlocked(value) })
}

func withPreferencesLock(action func() error) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := lockPreferences(file); err != nil {
		return err
	}
	defer unlockPreferences(file)
	return action()
}

func saveUnlocked(value Preferences) error {
	path, err := Path()
	if err != nil {
		return err
	}
	value = normalise(value)
	value.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".preferences-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return replaceFile(tmpPath, path)
}

func UpsertProject(root string, name string) (Preferences, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Preferences{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(root)
	}
	return updatePreferences(func(prefs *Preferences) error {
		prefs.LastProjectRoot = root
		now := time.Now().UTC()
		for idx := range prefs.Projects {
			if samePath(prefs.Projects[idx].Root, root) {
				prefs.Projects[idx].Root = root
				prefs.Projects[idx].Name = name
				prefs.Projects[idx].LastOpenedAt = now
				return nil
			}
		}
		prefs.Projects = append(prefs.Projects, ProjectRef{Root: root, Name: name, LastOpenedAt: now})
		return nil
	})
}

// RenameProject changes only TestDesk's display name. It never renames files or
// modifies the project configuration itself.
func RenameProject(root string, name string) (Preferences, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Preferences{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Preferences{}, errors.New("project display name is required")
	}
	return updatePreferences(func(prefs *Preferences) error {
		for index := range prefs.Projects {
			if samePath(prefs.Projects[index].Root, root) {
				prefs.Projects[index].Name = name
				return nil
			}
		}
		return errors.New("project is not known to TestDesk")
	})
}
func ForgetProject(root string) (Preferences, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Preferences{}, err
	}
	return updatePreferences(func(prefs *Preferences) error {
		kept := make([]ProjectRef, 0, len(prefs.Projects))
		for _, project := range prefs.Projects {
			if !samePath(project.Root, root) {
				kept = append(kept, project)
			}
		}
		prefs.Projects = kept
		if samePath(prefs.LastProjectRoot, root) {
			prefs.LastProjectRoot = ""
			if len(prefs.Projects) > 0 {
				prefs.LastProjectRoot = prefs.Projects[0].Root
			}
		}
		return nil
	})
}

func updatePreferences(update func(*Preferences) error) (Preferences, error) {
	var value Preferences
	err := withPreferencesLock(func() error {
		var err error
		value, err = Load()
		if err != nil {
			return err
		}
		if err = update(&value); err != nil {
			return err
		}
		value = normalise(value)
		return saveUnlocked(value)
	})
	return value, err
}

func normalise(value Preferences) Preferences {
	projectsByKey := map[string]ProjectRef{}
	for _, project := range value.Projects {
		root := strings.TrimSpace(project.Root)
		if root == "" {
			continue
		}
		absRoot, err := filepath.Abs(root)
		if err == nil {
			root = absRoot
		}
		key := pathKey(root)
		project.Root = root
		configPath := filepath.Join(root, "testdesk.json")
		if data, readErr := os.ReadFile(configPath); readErr == nil {
			var parsed struct {
				Name string `json:"name"`
			}
			if unmarshalErr := json.Unmarshal(data, &parsed); unmarshalErr == nil && strings.TrimSpace(parsed.Name) != "" {
				project.Name = strings.TrimSpace(parsed.Name)
			}
		}
		if strings.TrimSpace(project.Name) == "" {
			project.Name = filepath.Base(root)
		}
		existing, exists := projectsByKey[key]
		if !exists || project.LastOpenedAt.After(existing.LastOpenedAt) {
			projectsByKey[key] = project
		}
	}
	if strings.TrimSpace(value.LastProjectRoot) != "" {
		root := strings.TrimSpace(value.LastProjectRoot)
		absRoot, err := filepath.Abs(root)
		if err == nil {
			root = absRoot
		}
		key := pathKey(root)
		if existing, exists := projectsByKey[key]; exists {
			value.LastProjectRoot = existing.Root
		} else {
			projectsByKey[key] = ProjectRef{Root: root, Name: filepath.Base(root), LastOpenedAt: value.UpdatedAt}
			value.LastProjectRoot = root
		}
	}

	projects := make([]ProjectRef, 0, len(projectsByKey))
	for _, project := range projectsByKey {
		projects = append(projects, project)
	}
	sort.SliceStable(projects, func(i, j int) bool {
		return projects[i].LastOpenedAt.After(projects[j].LastOpenedAt)
	})
	value.Projects = projects
	if value.LastProjectRoot == "" && len(projects) > 0 {
		value.LastProjectRoot = projects[0].Root
	}
	return value
}

func samePath(left string, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	leftClean := filepath.Clean(strings.TrimSpace(left))
	rightClean := filepath.Clean(strings.TrimSpace(right))
	leftRoot, leftErr := filepath.Abs(leftClean)
	rightRoot, rightErr := filepath.Abs(rightClean)
	if leftErr == nil {
		leftClean = leftRoot
	}
	if rightErr == nil {
		rightClean = rightRoot
	}
	if leftClean == rightClean {
		return true
	}
	fi1, err1 := os.Stat(leftClean)
	fi2, err2 := os.Stat(rightClean)
	if err1 == nil && err2 == nil && os.SameFile(fi1, fi2) {
		return true
	}
	return strings.EqualFold(leftClean, rightClean)
}

func pathKey(value string) string {
	abs, err := filepath.Abs(strings.TrimSpace(value))
	if err == nil {
		value = abs
	}
	return strings.ToLower(filepath.Clean(value))
}
