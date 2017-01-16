package middleware

import (
	"fmt"
	"reflect"
	"runtime"

	. "github.com/getsentry/raven-go"
	"github.com/gobuffalo/buffalo"
	"github.com/pkg/errors"
)

// stackTracer exposes the StackTrace method in the errors pkg
type stackTracer interface {
	StackTrace() errors.StackTrace
}

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
					packet := NewPacket(rStr, NewException(errors.New(rStr), NewStacktrace(3, 3, prefixes)), NewHttp(c.Request()))
					Capture(packet, nil)
					panic(r)
				}
			}()
			err := next(c)
			if !panicsOnly && err != nil {
				httpErr, ok := err.(buffalo.HTTPError)
				if ok {
					err = httpErr.Cause
				}
				tracer, ok := err.(stackTracer)
				if ok {
					packet := NewPacket(err.Error(), NewException(httpErr, buildSentryStackTrace(tracer, prefixes)), NewHttp(c.Request()))
					Capture(packet, nil)
				} else {
					// if the error doesn't conform to the stackTracer interface then just send it along without a stack trace
					fmt.Println(reflect.TypeOf(err))
					packet := NewPacket(err.Error(), &Message{Message: err.Error()}, NewHttp(c.Request()))
					Capture(packet, nil)
				}
			}

			return err
		}
	}

}

func buildSentryStackTrace(tracer stackTracer, appPackagePrefixes []string) *Stacktrace {
	trace := []errors.Frame(tracer.StackTrace())
	// We aren't sure how much of our stack trace is going to pass the appPackagePrefix test
	var sentryFrames []*StacktraceFrame
	// Iterate through each stack frame and get the function
	// if we find a function get its file and line number
	// then call NewStackTraceFrames from Sentry to build a sentry frame
	for i := len(trace) - 1; i >= 0; i-- {
		fn := runtime.FuncForPC(pc(trace[i]))
		if fn == nil {
			continue
		}
		file, line := fn.FileLine(pc(trace[i]))
		frame := NewStacktraceFrame(pc(trace[i]), file, line, 3, appPackagePrefixes)
		if frame != nil {
			sentryFrames = append(sentryFrames, frame)
		}
	}
	return &Stacktrace{sentryFrames}
}

// pc recovers uintptrs from errors.Frames
func pc(frame errors.Frame) uintptr {
	return (uintptr(frame) - 1)
}
