package middleware

import (
	"fmt"
	"runtime"

	"github.com/getsentry/raven-go"
	"github.com/gobuffalo/buffalo"
	"github.com/pkg/errors"
)

// Sentry returns a piece of buffalo.Middleware that can
// be used to report exception to sentry. the sentry client must be initialized
// using raven.SetDSN() before use.  Accepts a list of package name prefixes such as
// github.com/myOrg/myApp to determine whether code is "in app", will re-issue all panics
func Sentry(prefixes []string, panicsOnly bool) buffalo.MiddlewareFunc {
	return func(next buffalo.Handler) buffalo.Handler {
		return func(c buffalo.Context) error {
			defer func() {
				if r := recover(); r != nil {
					rStr := fmt.Sprint(r)
					packet := raven.NewPacket(rStr, raven.NewException(errors.New(rStr), raven.NewStacktrace(3, 3, prefixes)), raven.NewHttp(c.Request()))
					raven.Capture(packet, nil)
					panic(r)
				}
			}()
			err := next(c)
			if !panicsOnly && err != nil {
				packet := buildErrPacket(err, prefixes, c)
				raven.Capture(packet, nil)
			}

			return err
		}
	}

}

func buildErrPacket(err error, prefixes []string, c buffalo.Context) *raven.Packet {

	// build a slice from the error chain to send to sentry
	chain := buffalo.ErrorChain(err)
	var sentryReports []raven.Interface
	sentryExceptions := raven.Exceptions{}
	// send errors to sentry in causal order
	for i := len(chain) - 1; i >= 0; i-- {
		sentryExceptions.Values = append(sentryExceptions.Values, raven.NewException(chain[i], buildSentryStackTrace(chain[i], prefixes)))
	}

	sentryReports = append(sentryReports, sentryExceptions)

	// add the http request context
	sentryReports = append(sentryReports, raven.NewHttp(c.Request()))
	packet := &raven.Packet{
		Message:    chain[len(chain)-1].Error(),
		Interfaces: sentryReports,
	}
	return packet
}

func buildSentryStackTrace(err error, appPackagePrefixes []string) *raven.Stacktrace {
	tracer, ok := err.(buffalo.StackTracer)
	// if the error doesn't have a StackTrace() method return nil
	if !ok {
		return nil
	}

	trace := []errors.Frame(tracer.StackTrace())
	// We aren't sure how much of our stack trace is going to pass the appPackagePrefix test
	var sentryFrames []*raven.StacktraceFrame
	// Iterate through each stack frame and get the function
	// if we find a function get its file and line number
	// then call NewStackTraceFrames from Sentry to build a sentry frame
	for i := len(trace) - 1; i >= 0; i-- {
		fn := runtime.FuncForPC(pc(trace[i]))
		if fn == nil {
			continue
		}
		file, line := fn.FileLine(pc(trace[i]))
		frame := raven.NewStacktraceFrame(pc(trace[i]), file, line, 3, appPackagePrefixes)
		if frame != nil {
			sentryFrames = append(sentryFrames, frame)
		}
	}
	return &raven.Stacktrace{sentryFrames}
}

// pc recovers uintptrs from errors.Frames
func pc(frame errors.Frame) uintptr {
	return (uintptr(frame) - 1)
}
