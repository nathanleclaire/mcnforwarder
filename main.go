package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"time"
)

type InspectAller interface {
	InspectAll() ([]byte, error)
}

type ContainerLister interface {
	ListContainers() ([]string, error)
}

type Container struct {
	NetworkSettings struct {
		Ports map[string][]struct {
			HostIp, HostPort string
		}
	}
}

type Forwarder interface {
	Forward(ports []string) error
}

type MachineForwarder struct {
	InspectAller
	ContainerLister
	Forwarder
	VMName, MachineBinary string
	SSHCmd                *exec.Cmd
}

func NewMachineForwarder(name string) *MachineForwarder {
	return &MachineForwarder{
		VMName:        name,
		MachineBinary: "docker-machine",
	}
}

func (fwder MachineForwarder) ListContainers() ([]string, error) {
	// Get all container IDs
	cmd := exec.Command(fwder.MachineBinary, "ssh", fwder.VMName, "docker", "ps", "-aq")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return strings.Fields(string(out)), nil
}

func (fwder MachineForwarder) InspectAll() ([]byte, error) {
	ids, err := fwder.ListContainers()
	if err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		return nil, nil
	}

	args := append([]string{"ssh", fwder.VMName, "docker", "inspect"}, ids...)

	cmd := exec.Command(fwder.MachineBinary, args...)

	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return out, nil
}

func (fwder MachineForwarder) Kill() error {
	if fwder.SSHCmd == nil {
		return nil
	}

	log.Print("Killing existing SSH process...")
	if err := fwder.SSHCmd.Process.Kill(); err != nil {
		return fmt.Errorf("Killing existing SSH process failed: %s", err)
	}
	return nil
}

func (fwder MachineForwarder) Forward(ports []string) error {
	// 'ssh' to designate the machine sub-command
	// VMName to specify the VM
	// '-N' to avoid spawning a shell when running this SSH command
	// '-vvv' to get lots of debugging output
	args := []string{"ssh", fwder.VMName, "-N", "-vvv", "-L", "2376:127.0.0.1:2376"}

	for _, port := range ports {
		args = append(args, "-L")
		args = append(args, fmt.Sprintf("%s:127.0.0.1:%s", port, port))
	}

	if fwder.SSHCmd != nil {
		if err := fwder.Kill(); err != nil {
			return err
		}
	}

	fwder.SSHCmd = exec.Command(fwder.MachineBinary, args...)

	log.Printf("Running SSH forwarding command %+v", fwder.SSHCmd)

	fwder.SSHCmd.Stdout = os.Stdout
	fwder.SSHCmd.Stderr = os.Stderr

	go fwder.SSHCmd.Run()

	return nil
}

func (fwder MachineForwarder) Poll(signalCh chan os.Signal) {
	forwardedPorts := []string{}
	changed := true

	if err := fwder.Forward(forwardedPorts); err != nil {
		log.Print("Error running forward: ", err)
		signalCh <- os.Interrupt
		return
	}

	for {
		inspectData, err := fwder.InspectAll()
		if err != nil {
			log.Print("Error inspecting: ", err)
			signalCh <- os.Interrupt
			return
		}

		if inspectData == nil {
			continue
		}

		newForwardedPorts := []string{}
		containers := []*Container{}

		if err := json.Unmarshal(inspectData, &containers); err != nil {
			log.Print("Error unmarshaling JSON: ", err)
			signalCh <- os.Interrupt
			return
		}

		for _, c := range containers {
			for _, port := range c.NetworkSettings.Ports {
				for _, info := range port {
					if info.HostIp == "127.0.0.1" || info.HostIp == "0.0.0.0" {
						newForwardedPorts = append(newForwardedPorts, info.HostPort)
					}
				}
			}
		}

		sort.Sort(sort.StringSlice(newForwardedPorts))

		if len(forwardedPorts) != len(newForwardedPorts) {
			changed = true
		} else {
			for index, val := range forwardedPorts {
				if newForwardedPorts[index] != val {
					changed = true
				}
			}
		}

		if changed {
			log.Print("Change detected, reloading...")
			log.Print(forwardedPorts)
			log.Print(newForwardedPorts)
			forwardedPorts = newForwardedPorts
			if err := fwder.Forward(forwardedPorts); err != nil {
				log.Print("Error running forward: ", err)
				signalCh <- os.Interrupt
				return
			}
		}

		changed = false
		time.Sleep(100 * time.Millisecond)
	}
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: mcnforwarder [name]")
	}

	signalCh := make(chan os.Signal, 1)
	cleanupDone := make(chan bool)
	signal.Notify(signalCh, os.Interrupt)

	go func() {
		log.Print("Starting Docker Machine SSH auto-forwarder...")
		fwder := NewMachineForwarder(os.Args[1])

		go fwder.Poll(signalCh)

		for _ = range signalCh {
			log.Print("Received SIGINT, cleaning up...")
			if err := fwder.Kill(); err != nil {
				log.Fatal("Error attempting cleanup: %s", err)
			}
			log.Print("Cleanup successful.")
			cleanupDone <- true
			return
		}
	}()

	<-cleanupDone
}
