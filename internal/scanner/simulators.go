package scanner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

type simulator struct {
	Name        string `json:"name"`
	UDID        string `json:"udid"`
	IsAvailable *bool  `json:"isAvailable"`
}

func simulatorCandidates(data []byte, root string) ([]Item, error) {
	var response struct {
		Devices map[string][]simulator `json:"devices"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	uuid := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	var items []Item
	for _, devices := range response.Devices {
		for _, device := range devices {
			if device.IsAvailable == nil || *device.IsAvailable || !uuid.MatchString(device.UDID) {
				continue
			}
			items = append(items, Item{Name: device.Name, Path: filepath.Join(root, device.UDID), Owner: "Xcode Simulator", Category: CatXcode, Safety: SafetyCaution, Source: "simulator-unavailable", Eligibility: Protected, Description: "Unavailable simulator. Inspect it in Xcode before removing its device data."})
		}
	}
	return items, nil
}

func discoverSimulators(ctx context.Context) ([]Item, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(home, "Library/Developer/CoreSimulator/Devices")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	data, err := exec.CommandContext(bounded, "/usr/bin/xcrun", "simctl", "list", "devices", "unavailable", "--json").Output()
	if err != nil {
		return []Item{{Path: root, Category: CatXcode}}, err
	}
	return simulatorCandidates(data, root)
}
