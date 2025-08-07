package runtime

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
)

type ContainerState string

const (
	StateCreating ContainerState = "creating"
	StateCreated  ContainerState = "created"
	StateRunning  ContainerState = "running"
	StateStopped  ContainerState = "stopped"
	StateError    ContainerState = "error"
)

type ContainerConfig struct {
	// commands to run inside the container
	Args []string
	// Env variables
	Env []string
	// Working directory
	WorkingDir string
	// Root filesystem path
	Rootfs string
	// Hostname inside container
	Hostname string
	// User to run as (format: "uid:gid")
	User string
}

type Process struct {
	Pid      int
	ExitCode int
	Started  time.Time
	Finished time.Time      // zero if still running
	Signal   syscall.Signal // signal used to kill the process (0 if exited normally)
}

type Container struct {
	ID string

	// configuration used to create this container
	Config *ContainerConfig

	// current state of the container
	state      ContainerState
	stateMutex sync.RWMutex

	// main process running in the container
	process      *Process
	processMutex sync.RWMutex

	// Channel to signal container should stop
	stopCh chan struct{}

	// Channel that closes when container exits
	exitCh chan struct{}

	// error that occured during container lifecycle
	lastError  error
	errorMutex sync.RWMutex
}

func NewContainer(id string, config *ContainerConfig) *Container {
	return &Container{
		ID:     id,
		Config: config,
		state:  StateCreating,
		stopCh: make(chan struct{}),
		exitCh: make(chan struct{}),
	}
}

// returns the current state of the container (thread-safe)
func (c *Container) State() ContainerState {
	c.stateMutex.RLock()
	defer c.stateMutex.RUnlock()
	return c.state
}

// updates the container state (internal use)
func (c *Container) setState(state ContainerState) {
	c.stateMutex.Lock()
	defer c.stateMutex.Unlock()
	c.state = state
}

// returns container's main process (thread safe)
func (c *Container) Process() *Process {
	c.processMutex.RLock()
	defer c.processMutex.RUnlock()
	if c.process == nil {
		return nil
	}

	// Return a copy to prevent external modification.
	return &Process{
		Pid:      c.process.Pid,
		ExitCode: c.process.ExitCode,
		Started:  c.process.Started,
		Finished: c.process.Finished,
		Signal:   c.process.Signal,
	}
}

func (c *Container) setProcess(process *Process) {
	c.processMutex.Lock()
	defer c.processMutex.Unlock()
	c.process = process
}

func (c *Container) setError(err error) {
	c.errorMutex.Lock()
	c.lastError = err
	c.errorMutex.Unlock()
	c.setState(StateError)
}

func (c *Container) validateConfig() error {
	if c.Config == nil {
		return fmt.Errorf("container configuration can't be nil")
	}

	if len(c.Config.Args) == 0 {
		return fmt.Errorf("no command specified")
	}

	if _, err := os.Stat(c.Config.Args[0]); err != nil {
		return fmt.Errorf("command %s not found: %w", c.Config.Args[0], err)
	}

	// Validate rootfs path if specified
	if c.Config.Rootfs != "" {
		if info, err := os.Stat(c.Config.Rootfs); err != nil {
			return fmt.Errorf("rootfs path %s not found: %w", c.Config.Rootfs, err)
		} else if !info.IsDir() {
			return fmt.Errorf("rootfs path %s is not a directory", c.Config.Rootfs)
		}

	}

	return nil
}

func (c *Container) monitorProcess() {
	defer close(c.exitCh)

	process := c.Process()

	if process == nil {
		c.setError(fmt.Errorf("no process to monitor"))
		return
	}

	// wait for the process to exit
	// TODO: This will be implemented when we execute the actuall process
	// for now this is a place holder

	// when process exits, update state
	c.setState(StateStopped)
}

// Create Prepares the container but doesn't start it
// This is the place we set up namespaces, cgroups, filesystem
func (c *Container) Create() error {
	if c.State() != StateCreating {
		return fmt.Errorf("rontainer %s is not creating state", c.ID)
	}

	if err := c.validateConfig(); err != nil {
		c.setError(err)
		return fmt.Errorf("invalid container configuration: %w", err)
	}

	// TODO: Set up namespaces
	// TODO: Set up cgroups
	// TODO: Set up filesystem
	// TODO: Prepare container environment

	c.setState(StateCreated)
	return nil
}

// Start begins execution of the container's main process
// The container must be in Created state
func (c *Container) Start() error {
	if c.State() != StateCreated {
		return fmt.Errorf("container %s is not in created state", c.ID)
	}

	// TODO: This is where we'll implement the low-level process creation
	// using clone() syscall

	c.setState(StateRunning)

	go c.monitorProcess()

	return nil
}

// Wait blocks until container exits and return the exit code
func (c *Container) Wait() (int, error) {
	<-c.exitCh

	process := c.Process()
	if process == nil {
		return -1, fmt.Errorf("container %s has no process information", c.ID)
	}
	return process.ExitCode, c.lastError
}

// Delete cleans up all resources associated with the container
func (c *Container) Delete() error {
	state := c.State()
	if state == StateRunning {
		return fmt.Errorf("cannot delete running container %s", c.ID)
	}

	// TODO: Clean up namespaces
	// TODO: Clean up cgroups
	// TODO: Clean up filesystem mounts
	// TODO: Remove container root directory

	return nil
}

// Kill forcully terminates the container process with SIGKILL
func (c *Container) Kill() error {
	process := c.Process()
	if process == nil {
		return fmt.Errorf("container %s has no running process", c.ID)
	}

	if err := syscall.Kill(process.Pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("failed to send SIGKILL to process %d: %w", process.Pid, err)
	}

	return nil
}
