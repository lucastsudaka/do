package do

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	t.Parallel()
	is := assert.New(t)

	i := New()
	is.NotNil(i)

	is.NotNil(i.opts.Logf)
	is.Nil(i.opts.HookAfterRegistration)
	is.Nil(i.opts.HookAfterShutdown)
	is.Empty(i.opts.HealthCheckParallelism)
	is.Empty(i.opts.HealthCheckGlobalTimeout)
	is.Empty(i.opts.HealthCheckTimeout)

	is.Nil(i.healthCheckPool)
	is.NotNil(i.self)
	is.Equal("[root]", i.self.name)
	is.Equal(i.self.rootScope, i)
	is.Nil(i.self.parentScope)
}

func TestNewWithOpts(t *testing.T) {
	t.Parallel()
	is := assert.New(t)

	i := NewWithOpts(&InjectorOpts{
		HookAfterRegistration: func(scope *Scope, serviceName string) {},
		HookAfterShutdown:     func(scope *Scope, serviceName string) {},
		Logf:                  func(format string, args ...any) {},

		HealthCheckParallelism:   42,
		HealthCheckGlobalTimeout: 42 * time.Second,
		HealthCheckTimeout:       42 * time.Second,
	})
	defer i.Shutdown() // nolint: errcheck

	is.NotNil(i)

	is.NotNil(i.opts.HookAfterRegistration)
	is.NotNil(i.opts.HookAfterShutdown)
	is.NotNil(i.opts.Logf)
	is.EqualValues(42, i.opts.HealthCheckParallelism)
	is.EqualValues(42*time.Second, i.opts.HealthCheckGlobalTimeout)
	is.EqualValues(42*time.Second, i.opts.HealthCheckTimeout)

	is.NotNil(i.healthCheckPool)
	is.NotNil(i.self)
	is.Equal("[root]", i.self.name)
	is.Equal(i.self.rootScope, i)
	is.Nil(i.self.parentScope)
}

func TestRootScope_RootScope(t *testing.T) {
	t.Parallel()
	is := assert.New(t)

	i := New()
	is.Equal(i, i.RootScope())
}

func TestRootScope_Ancestors(t *testing.T) {
	t.Parallel()
	is := assert.New(t)

	i := New()
	is.Len(i.Ancestors(), 0)
}

func TestRootScope_queueServiceHealthcheck(t *testing.T) {
	testWithTimeout(t, 200*time.Millisecond)
	is := assert.New(t)

	// no timeout
	i := New()
	ProvideValue[*lazyTestHeathcheckerOKTimeout](i, &lazyTestHeathcheckerOKTimeout{foobar: "foobar"})
	ProvideValue[*lazyTestHeathcheckerOK](i, &lazyTestHeathcheckerOK{})

	err1 := i.queueServiceHealthcheck(context.Background(), i.self, NameOf[*lazyTestHeathcheckerOKTimeout]())
	err2 := i.queueServiceHealthcheck(context.Background(), i.self, NameOf[*lazyTestHeathcheckerOK]())
	is.Nil(<-err1)
	is.Nil(<-err2)

	// with 10ms individual timeout
	i = NewWithOpts(&InjectorOpts{
		HealthCheckTimeout: 10 * time.Millisecond,
	})
	ProvideValue[*lazyTestHeathcheckerOKTimeout](i, &lazyTestHeathcheckerOKTimeout{})
	ProvideValue[*lazyTestHeathcheckerOK](i, &lazyTestHeathcheckerOK{})

	err1 = i.queueServiceHealthcheck(context.Background(), i.self, NameOf[*lazyTestHeathcheckerOKTimeout]())
	err2 = i.queueServiceHealthcheck(context.Background(), i.self, NameOf[*lazyTestHeathcheckerOK]())
	is.EqualError(<-err1, "DI: health check timeout: context deadline exceeded")
	is.Nil(<-err2)

	// with 10ms global timeout
	i = NewWithOpts(&InjectorOpts{
		HealthCheckGlobalTimeout: 10 * time.Millisecond,
	})
	ProvideValue[*lazyTestHeathcheckerOKTimeout](i, &lazyTestHeathcheckerOKTimeout{})
	ProvideValue[*lazyTestHeathcheckerOK](i, &lazyTestHeathcheckerOK{})

	errAll := i.HealthCheckWithContext(context.Background())
	is.Len(errAll, 2)
	is.EqualError(errAll[NameOf[*lazyTestHeathcheckerOKTimeout]()], "DI: health check timeout: context deadline exceeded")
	is.Nil(errAll[NameOf[*lazyTestHeathcheckerOK]()])

	// with 10ms global timeout with sequential healthchecks
	i = NewWithOpts(&InjectorOpts{
		HealthCheckParallelism:   1,
		HealthCheckGlobalTimeout: 50 * time.Millisecond,
	})
	defer i.Shutdown() // nolint: errcheck

	ProvideNamedValue[*lazyTestHeathcheckerOKTimeout](i, "a", &lazyTestHeathcheckerOKTimeout{})
	ProvideNamedValue[*lazyTestHeathcheckerOKTimeout](i, "b", &lazyTestHeathcheckerOKTimeout{})
	ProvideNamedValue[*lazyTestHeathcheckerOKTimeout](i, "c", &lazyTestHeathcheckerOKTimeout{})
	ProvideNamedValue[*lazyTestHeathcheckerOKTimeout](i, "d", &lazyTestHeathcheckerOKTimeout{})
	ProvideValue[*lazyTestHeathcheckerOK](i, &lazyTestHeathcheckerOK{})

	errAll = i.HealthCheckWithContext(context.Background())
	errors := values(errAll)
	errors = filter(errors, func(item error, index int) bool {
		return item != nil
	})

	is.Len(errAll, 5)
	// i do not check the exact number of errors due to sleep randomness
	is.Greater(len(errors), 0)
	is.Less(len(errors), 5)
	if len(errors) == 3 {
		is.EqualError(errors[0], "DI: health check timeout: context deadline exceeded")
	}
	// because executed last
	// is.EqualError(errAll[NameOf[*lazyTestHeathcheckerOK]()], "DI: health check timeout: context deadline exceeded")
}

func TestRootScope_Clone(t *testing.T) {
	is := assert.New(t)

	opts := &InjectorOpts{
		HookAfterRegistration: func(scope *Scope, serviceName string) {},
		HookAfterShutdown:     func(scope *Scope, serviceName string) {},
		Logf:                  func(format string, args ...any) {},

		HealthCheckParallelism:   42,
		HealthCheckGlobalTimeout: 42 * time.Second,
		HealthCheckTimeout:       42 * time.Second,
	}

	i := NewWithOpts(opts)
	clone := i.Clone()

	defer i.Shutdown()     // nolint: errcheck
	defer clone.Shutdown() // nolint: errcheck

	is.Equal(i.opts, clone.opts)

	is.NotNil(i.opts.HookAfterRegistration)
	is.NotNil(i.opts.HookAfterShutdown)
	is.NotNil(i.opts.Logf)
	is.NotNil(i.opts.HealthCheckParallelism)
	is.NotNil(i.opts.HealthCheckGlobalTimeout)
	is.NotNil(i.opts.HealthCheckTimeout)

	is.NotNil(clone.opts.HookAfterRegistration)
	is.NotNil(clone.opts.HookAfterShutdown)
	is.NotNil(clone.opts.Logf)
	is.NotNil(clone.opts.HealthCheckParallelism)
	is.NotNil(clone.opts.HealthCheckGlobalTimeout)
	is.NotNil(clone.opts.HealthCheckTimeout)

	is.EqualValues(42, clone.opts.HealthCheckParallelism)
	is.EqualValues(42*time.Second, clone.opts.HealthCheckGlobalTimeout)
	is.EqualValues(42*time.Second, clone.opts.HealthCheckTimeout)
	is.EqualValues(i.opts.HealthCheckParallelism, clone.opts.HealthCheckParallelism)
	is.EqualValues(i.opts.HealthCheckGlobalTimeout, clone.opts.HealthCheckGlobalTimeout)
	is.EqualValues(i.opts.HealthCheckTimeout, clone.opts.HealthCheckTimeout)
}

func TestRootScope_CloneWithOpts(t *testing.T) {
	is := assert.New(t)

	i := New()
	clone := i.CloneWithOpts(&InjectorOpts{
		HookAfterRegistration: func(scope *Scope, serviceName string) {},
		HookAfterShutdown:     func(scope *Scope, serviceName string) {},
		Logf:                  func(format string, args ...any) {},

		HealthCheckParallelism:   42,
		HealthCheckGlobalTimeout: 42 * time.Second,
		HealthCheckTimeout:       42 * time.Second,
	})

	defer i.Shutdown()     // nolint: errcheck
	defer clone.Shutdown() // nolint: errcheck

	is.Nil(i.opts.HookAfterRegistration)
	is.Nil(i.opts.HookAfterShutdown)
	is.NotNil(i.opts.Logf)
	is.Empty(i.opts.HealthCheckParallelism)
	is.Empty(i.opts.HealthCheckGlobalTimeout)
	is.Empty(i.opts.HealthCheckTimeout)

	is.NotNil(clone.opts.HookAfterRegistration)
	is.NotNil(clone.opts.HookAfterShutdown)
	is.NotNil(clone.opts.Logf)
	is.Equal(uint(42), clone.opts.HealthCheckParallelism)
	is.Equal(42*time.Second, clone.opts.HealthCheckGlobalTimeout)
	is.Equal(42*time.Second, clone.opts.HealthCheckTimeout)

	// scope must be added only to initial scope
	i.Scope("foobar")
	is.Len(i.Children(), 1)
	is.Len(clone.Children(), 0)

	is.Nil(i.healthCheckPool)
	is.NotNil(clone.healthCheckPool)
}

func TestRootScope_ShutdownOnSignals(t *testing.T) {
	// @TODO
}

func TestRootScope_ShutdownOnSignalsWithContext(t *testing.T) {
	// @TODO
}
