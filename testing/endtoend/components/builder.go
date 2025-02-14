package components

import (
	"context"
	"fmt"
	"os"
	"path"
	"strconv"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/testing/endtoend/helpers"
	e2e "github.com/theQRL/qrysm/testing/endtoend/params"
	e2etypes "github.com/theQRL/qrysm/testing/endtoend/types"
	"github.com/theQRL/qrysm/testing/middleware/builder"
)

// BuilderSet represents a set of builders for the validators running via a relay.
type BuilderSet struct {
	e2etypes.ComponentRunner
	started  chan struct{}
	builders []e2etypes.ComponentRunner
}

// NewBuilderSet creates and returns a set of builders.
func NewBuilderSet() *BuilderSet {
	return &BuilderSet{
		started: make(chan struct{}, 1),
	}
}

// Start starts all the builders in set.
func (s *BuilderSet) Start(ctx context.Context) error {
	totalNodeCount := e2e.TestParams.BeaconNodeCount
	nodes := make([]e2etypes.ComponentRunner, totalNodeCount)
	for i := 0; i < totalNodeCount; i++ {
		nodes[i] = NewBuilder(i)
	}
	s.builders = nodes

	// Wait for all nodes to finish their job (blocking).
	// Once nodes are ready passed in handler function will be called.
	return helpers.WaitOnNodes(ctx, nodes, func() {
		// All nodes started, close channel, so that all services waiting on a set, can proceed.
		close(s.started)
	})
}

// Started checks whether builder set is started and all builders are ready to be queried.
func (s *BuilderSet) Started() <-chan struct{} {
	return s.started
}

// Pause pauses the component and its underlying process.
func (s *BuilderSet) Pause() error {
	for _, n := range s.builders {
		if err := n.Pause(); err != nil {
			return err
		}
	}
	return nil
}

// Resume resumes the component and its underlying process.
func (s *BuilderSet) Resume() error {
	for _, n := range s.builders {
		if err := n.Resume(); err != nil {
			return err
		}
	}
	return nil
}

// Stop stops the component and its underlying process.
func (s *BuilderSet) Stop() error {
	for _, n := range s.builders {
		if err := n.Stop(); err != nil {
			return err
		}
	}
	return nil
}

// PauseAtIndex pauses the component and its underlying process at the desired index.
func (s *BuilderSet) PauseAtIndex(i int) error {
	if i >= len(s.builders) {
		return errors.Errorf("provided index exceeds slice size: %d >= %d", i, len(s.builders))
	}
	return s.builders[i].Pause()
}

// ResumeAtIndex resumes the component and its underlying process at the desired index.
func (s *BuilderSet) ResumeAtIndex(i int) error {
	if i >= len(s.builders) {
		return errors.Errorf("provided index exceeds slice size: %d >= %d", i, len(s.builders))
	}
	return s.builders[i].Resume()
}

// StopAtIndex stops the component and its underlying process at the desired index.
func (s *BuilderSet) StopAtIndex(i int) error {
	if i >= len(s.builders) {
		return errors.Errorf("provided index exceeds slice size: %d >= %d", i, len(s.builders))
	}
	return s.builders[i].Stop()
}

// ComponentAtIndex returns the component at the provided index.
func (s *BuilderSet) ComponentAtIndex(i int) (e2etypes.ComponentRunner, error) {
	if i >= len(s.builders) {
		return nil, errors.Errorf("provided index exceeds slice size: %d >= %d", i, len(s.builders))
	}
	return s.builders[i], nil
}

// Builder represents a block builder.
type Builder struct {
	e2etypes.ComponentRunner
	started chan struct{}
	index   int
	builder *builder.Builder
	cancel  func()
}

// NewBuilder creates and returns a builder.
func NewBuilder(index int) *Builder {
	return &Builder{
		started: make(chan struct{}, 1),
		index:   index,
	}
}

// Start runs a builder.
func (node *Builder) Start(ctx context.Context) error {
	f, err := os.Create(path.Join(e2e.TestParams.LogPath, "builder_"+strconv.Itoa(node.index)+".log"))
	if err != nil {
		return err
	}
	jwtPath := path.Join(e2e.TestParams.TestPath, "zonddata/"+strconv.Itoa(node.index)+"/")
	jwtPath = path.Join(jwtPath, "gzond/jwtsecret")
	secret, err := parseJWTSecretFromFile(jwtPath)
	if err != nil {
		return err
	}
	opts := []builder.Option{
		builder.WithDestinationAddress(fmt.Sprintf("http://127.0.0.1:%d", e2e.TestParams.Ports.GzondExecutionNodeAuthRPCPort+node.index)),
		builder.WithPort(e2e.TestParams.Ports.ProxyPort + node.index),
		builder.WithLogger(logrus.New()),
		builder.WithLogFile(f),
		builder.WithJwtSecret(string(secret)),
	}
	bd, err := builder.New(opts...)
	if err != nil {
		return err
	}
	log.Infof("Starting builder %d with port: %d and file %s", node.index, e2e.TestParams.Ports.ProxyPort+node.index, f.Name())

	// Set cancel into context.
	ctx, cancel := context.WithCancel(ctx)
	node.cancel = cancel
	node.builder = bd
	// Mark node as ready.
	close(node.started)
	return bd.Start(ctx)
}

// Started checks whether the builder is started and ready to be queried.
func (node *Builder) Started() <-chan struct{} {
	return node.started
}

// Pause pauses the component and its underlying process.
func (node *Builder) Pause() error {
	// no-op
	return nil
}

// Resume resumes the component and its underlying process.
func (node *Builder) Resume() error {
	// no-op
	return nil
}

// Stop kills the component and its underlying process.
func (node *Builder) Stop() error {
	node.cancel()
	return nil
}
