package middleware

import (
	"errors"
	"fmt"

	. "github.com/getsentry/raven-go"
	"github.com/gobuffalo/buffalo"
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
					packet := NewPacket(rStr, NewException(errors.New(rStr), NewStacktrace(3, 3, prefixes)), NewHttp(c.Request()))
					Capture(packet, nil)
					panic(r)
				}
			}()
			err := next(c)
			if !panicsOnly && err != nil {
				packet := NewPacket(err.Error(), NewException(err, NewStacktrace(2, 3, prefixes)), NewHttp(c.Request()))
				Capture(packet, nil)
			}
			return err
		}
	}

}
