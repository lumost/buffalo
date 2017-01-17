package buffalo

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gobuffalo/velvet"
	"github.com/pkg/errors"
)

// StackTracer exposes the StackTrace() method in the errors pkg
type StackTracer interface {
	StackTrace() errors.StackTrace
}

// Causer exposes the Cause() method of the errors package
type Causer interface {
	Cause() error
}

// HTTPError a typed error returned by http Handlers and used for choosing error handlers
type HTTPError struct {
	Status   int   `json:"status"`
	CausedBy error `json:"error"`
}

func (h HTTPError) Error() string {
	return h.CausedBy.Error()
}

func (h HTTPError) Cause() error {
	return h.CausedBy
}

// ErrorChain converts nested errors into a slice of errors
// the slice is returned with the oldest error last.
func ErrorChain(e error) []error {
	var causeChain []error
	cause, ok := e.(Causer)
	for ok {
		causeChain = append(causeChain, cause.(error))
		cause, ok = cause.Cause().(Causer)
	}
	// check if the last error in the cause chain is not nil and append as we are not
	// guaranteed that the final error will implement Causer
	if cause != nil {
		causeChain = append(causeChain, cause.(error))
	}
	return causeChain
}

// ErrorHandler interface for handling an error for a
// specific status code.
type ErrorHandler func(int, error, Context) error

// ErrorHandlers is used to hold a list of ErrorHandler
// types that can be used to handle specific status codes.
/*
	a.ErrorHandlers[500] = func(status int, err error, c buffalo.Context) error {
		res := c.Response()
		res.WriteHeader(status)
		res.Write([]byte(err.Error()))
		return nil
	}
*/
type ErrorHandlers map[int]ErrorHandler

// Get a registered ErrorHandler for this status code. If
// no ErrorHandler has been registered, a default one will
// be returned.
func (e ErrorHandlers) Get(status int) ErrorHandler {
	if eh, ok := e[status]; ok {
		return eh
	}
	return defaultErrorHandler
}

// unexported type used to handle errors with stack traces
type ErrorStack struct {
	Msg      string
	Stack    string
	HasStack bool
}

func defaultErrorHandler(status int, err error, c Context) error {
	env := c.Get("env")
	if env != nil && env.(string) == "production" {
		c.Response().WriteHeader(status)
		c.Response().Write([]byte(prodErrorTmpl))
		return nil
	}
	c.Logger().Error(err)
	c.Response().WriteHeader(status)

	// get the full error causal chain
	errorSlice := ErrorChain(err)
	var eStacks []ErrorStack
	for _, item := range errorSlice {
		tracer, ok := item.(StackTracer)
		var eStack ErrorStack
		if ok {
			stack := fmt.Sprintf("%+v", tracer.StackTrace())
			eStack = ErrorStack{
				Msg:      item.Error(),
				Stack:    stack,
				HasStack: true,
			}
		} else {
			eStack = ErrorStack{
				Msg:      item.Error(),
				HasStack: false,
			}
		}

		eStacks = append(eStacks, eStack)
	}
	// reverse the error slice to be oldest first
	for i := len(eStacks)/2 - 1; i >= 0; i-- {
		opp := len(eStacks) - 1 - i
		eStacks[i], eStacks[opp] = eStacks[opp], eStacks[i]
	}

	ct := c.Request().Header.Get("Content-Type")
	switch strings.ToLower(ct) {
	case "application/json", "text/json", "json":
		err = json.NewEncoder(c.Response()).Encode(map[string]interface{}{
			"errors": eStacks,
			"code":   status,
		})
	case "application/xml", "text/xml", "xml":
	default:
		data := map[string]interface{}{
			"routes": c.Get("routes"),
			"errors": eStacks,
			"status": status,
			"data":   c.Data(),
		}
		ctx := velvet.NewContextWith(data)
		t, err := velvet.Render(devErrorTmpl, ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		res := c.Response()
		res.WriteHeader(404)
		_, err = res.Write([]byte(t))
		return err
	}
	return err
}

var devErrorTmpl = `
<html>
<head>
	<title>{{status}} - ERROR!</title>
	<style>
		body {
			font-family: helvetica;
		}
		table {
			width: 100%;
		}
		th {
			text-align: left;
		}
		tr:nth-child(even) {
		  background-color: #dddddd;
		}
		td {
			margin: 0px;
			padding: 10px;
		}
		pre {
			display: block;
			padding: 9.5px;
			margin: 0 0 10px;
			font-size: 13px;
			line-height: 1.42857143;
			color: #333;
			word-break: break-all;
			word-wrap: break-word;
			background-color: #f5f5f5;
			border: 1px solid #ccc;
			border-radius: 4px;
		}
	</style>
</head>
<body>
<h1>{{status}} - ERROR!</h1>
{{#each errors as |error|}}
<pre>{{ error.Msg }}</pre>
{{#if error.HasStack }}
<pre>{{ error.Stack }}</pre>
{{/if}}
{{/each}}
<hr>
<h3>Context</h3>
<pre>{{#each data as |k v|}}
{{inspect k}}: {{inspect v}}
{{/each}}</pre>
<hr>
<h3>Routes</h3>
<table id="buffalo-routes-table">
	<thead>
		<tr>
			<th>METHOD</th>
			<th>PATH</th>
			<th>HANDLER</th>
		</tr>
	</thead>
	<tbody>
		{{#each routes as |route|}}
			<tr>
				<td>{{route.Method}}</td>
				<td>{{route.Path}}</td>
				<td><code>{{route.HandlerName}}</code></td>
			</tr>
		{{/each}}
	</tbody>
</table>
</body>
</html>
`
var prodErrorTmpl = `
<h1>We're Sorry!</h1>
<p>
It looks like something went wrong! Don't worry, we are aware of the problem and are looking into it.
</p>
<p>
Sorry if this has caused you any problems. Please check back again later.
</p>
`
