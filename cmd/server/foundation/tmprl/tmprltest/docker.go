package tmprltest

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"time"
)

// Keep the image and container name in sync with the tmprl Make targets.
const image = "temporalio/temporal:1.9.1"
const containerName = "moxie-tmprl-test"
const ownerLabel = "com.ardanlabs.moxie.tmprl-test"

func docker(ctx context.Context, args ...string) ([]byte, error) {
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker %v: %w: %s", args, err, output)
	}
	return output, nil
}

// ensureContainer leaves shared infrastructure running. Docker's unique name
// arbitrates creation across independent go test package processes.
func ensureContainer(ctx context.Context) (string, error) {
	if _, err := docker(ctx, "info", "--format", "{{.ServerVersion}}"); err != nil {
		return "", fmt.Errorf("start Docker before running Temporal tests: %w", err)
	}
	var lastErr error
	for range 10 {
		output, err := docker(ctx, "inspect", containerName)
		if err != nil {
			_, lastErr = docker(ctx, "run", "-d", "--name", containerName,
				"--label", ownerLabel+"=true", "-p", "127.0.0.1::7233", image,
				"server", "start-dev", "--ip", "0.0.0.0", "--headless",
				"--db-filename", "/tmp/temporal.db")
		} else {
			var containers []struct {
				Config struct {
					Image  string
					Labels map[string]string
				}
				State           struct{ Running bool }
				NetworkSettings struct {
					Ports map[string][]struct{ HostIP, HostPort string }
				}
			}
			if err := json.Unmarshal(output, &containers); err != nil {
				return "", err
			}
			if len(containers) != 1 {
				return "", fmt.Errorf("expected one %s container", containerName)
			}
			c := containers[0]
			if c.Config.Image != image || c.Config.Labels[ownerLabel] != "true" {
				return "", fmt.Errorf("container %s is not our %s test server; refusing to reuse or change it", containerName, image)
			}
			if c.State.Running {
				for _, binding := range c.NetworkSettings.Ports["7233/tcp"] {
					if binding.HostIP == "127.0.0.1" && binding.HostPort != "" {
						return net.JoinHostPort(binding.HostIP, binding.HostPort), nil
					}
				}
				lastErr = fmt.Errorf("container %s has no localhost Temporal port", containerName)
			} else {
				_, lastErr = docker(ctx, "start", containerName)
			}
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("starting Temporal: %w (%v)", ctx.Err(), lastErr)
		case <-time.After(250 * time.Millisecond):
		}
	}
	return "", fmt.Errorf("starting Temporal container: %v", lastErr)
}
