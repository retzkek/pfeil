package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	flag "github.com/spf13/pflag"
	jaeger "github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
)

var (
	version = "0.3.1"
	verbose = flag.BoolP("verbose", "v", false, "enable verbose/debug logging (default false)")
	sample  = flag.BoolP("sample", "y", false, "always sample new trace (sets SAMPLER_TYPE=const, SAMPLER_PARAM=1)")
	noop    = flag.BoolP("nosample", "n", false, "never sample new trace (overrides -y, sets SAMPLER_PARAM=0)")
	service = flag.StringP("service", "s", "", "service name for trace, overrides JAEGER_SERVICE_NAME")
	tags    = flag.StringSliceP("tag", "t", []string{}, `tags to include in span, as "key=value".
Multiple tags can be specified comma-separated, i.e. "k1=v1,k2=v2",
or the option can be repeated, i.e. "-t k1=v1 -t k2=v2".`)
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Pfeil version %s
Usage: %s [OPTS] OPERATION [CMD [ARGS...]]

Pfeil is a tool to send an OpenTracing span to a Jaeger agent or collector.

A service name must be set via --service/-s option or JAEGER_SERVICE_NAME.
The span operation name must be provided as the OPERATION argument. Use the
following environment variables to control the span:

  TRACE_ID       uber-trace-id, extracted from parent process or Pfeil run,
                 e.g. "13df7cb5f11aa574:13df7cb5f11aa574:0000000000000000:1"
  TRACE_START    timestamp in Unix date format to record for start of span

Tags can be provided as key=value pairs with the --tag/-t option. The new trace
ID will be printed to stdout so it can be used as the parent for child spans.

If the optional CMD and ARGS arguments are provided, CMD will be run with ARGS
in a subprocess, with stdin, stout, and stderr piped through. The exit code will
be added as tag exit_code, if it's nonzero then "error=true" will also be set.

Use the following environment variables to configure Jaeger (common settings,
see https://github.com/jaegertracing/jaeger-client-go for the full list):

  JAEGER_SERVICE_NAME      The service name (overriden by -svc).
  JAEGER_AGENT_HOST        The hostname for communicating with agent via UDP
                           (default localhost).
  JAEGER_AGENT_PORT        The port for communicating with agent via UDP
                           (default 6831).
  JAEGER_ENDPOINT          The HTTP endpoint for sending spans directly to a
                           collector, e.g.
                           http://jaeger-collector:14268/api/traces.
                           If specified, the agent host/port are ignored.
  JAEGER_SAMPLER_TYPE      The sampler type: remote, const, probabilistic,
                           ratelimiting (default remote). See -y and -n options
                           to conveniently set const sampling.
  JAEGER_SAMPLER_PARAM     The sampler parameter (number).
  JAEGER_SAMPLING_ENDPOINT The URL for the sampling configuration server when
                           using sampler type remote
                           (default http://127.0.0.1:5778/sampling).

Options:
`, version, os.Args[0])
		flag.PrintDefaults()
	}
}

var (
	l  = log.New(os.Stderr, "pfeil: ", log.LstdFlags|log.Lmsgprefix)
	vl = log.New(os.Stderr, "pfeil: ", log.LstdFlags|log.Lmsgprefix)
)

func main() {
	flag.Parse()

	if !*verbose {
		vl.SetOutput(ioutil.Discard)
	}

	if flag.NArg() == 0 {
		l.Fatalf("error: operation name argument must be set")
	}

	// load jaeger config from environment
	cfg, err := jaegercfg.FromEnv()
	if err != nil {
		l.Fatalf("error loading jaeger config from environment: %s", err)
	}
	// overrides from command options
	if *service != "" {
		cfg.ServiceName = *service
	}
	if cfg.ServiceName == "" {
		l.Fatalf("service name must be specified by one of JAEGER_SERVICE_NAME or -svc")
	}
	if *sample {
		cfg.Sampler.Type = jaeger.SamplerTypeConst
		cfg.Sampler.Param = 1
	}
	if *noop {
		cfg.Sampler.Type = jaeger.SamplerTypeConst
		cfg.Sampler.Param = 0
	}

	tracer, closer, err := cfg.NewTracer()
	if err != nil {
		l.Fatalf("could not initialize tracer: %s", err.Error())
	}
	defer closer.Close()
	opentracing.SetGlobalTracer(tracer)

	// get context from env vars
	opts := make([]opentracing.StartSpanOption, 0, 2)
	// TRACE_ID should be the uber-trace-id extracted via TextMap
	if traceid, found := os.LookupEnv("TRACE_ID"); found {
		vl.Printf("found TRACE_ID %s", traceid)
		textmap := map[string]string{"uber-trace-id": traceid}
		if spanCtx, err := tracer.Extract(opentracing.TextMap, opentracing.TextMapCarrier(textmap)); err == nil {
			opts = append(opts, ext.RPCServerOption(spanCtx))
		} else {
			vl.Printf("error injecting trace context: %s", err)
		}
	} else {
		vl.Print("no TRACE_ID found in env, starting new trace")
	}
	// TRACE_START records when the span started, in Unix date format.
	if start, found := os.LookupEnv("TRACE_START"); found {
		vl.Printf("found TRACE_START %s", start)
		if startt, err := time.Parse(time.UnixDate, start); err == nil {
			opts = append(opts, opentracing.StartTime(startt))
		} else {
			vl.Printf("error parsing TRACE_START: %s", err)
		}
	} else {
		vl.Print("no TRACE_START found in env, using now")
	}

	span := tracer.StartSpan(flag.Arg(0), opts...)
	defer span.Finish()
	vl.Printf("started span %s", span)
	if !span.Context().(jaeger.SpanContext).IsSampled() {
		vl.Printf("warning, trace not sampled")
	}

	// execute command if provided
	if flag.NArg() > 1 {
		// we could include the span in the context, but we need to modify it,
		// so we need to pass the span object itself instead
		ctx := context.Background()
		if err = runCommand(ctx, span, flag.Arg(1), flag.Args()[2:]...); err != nil {
			span.SetTag("error", true)
			err = fmt.Errorf("error running command: %s", err)
			span.SetTag("message", err.Error())
			l.Fatalf("error running command: %s", err)
		}
	}

	// parse tags from options
	for _, tag := range *tags {
		kv := strings.SplitN(tag, "=", 2)
		if len(kv) == 2 {
			vl.Printf("setting tag %s to \"%s\"", kv[0], kv[1])
			span.SetTag(kv[0], kv[1])
		} else {
			l.Printf("unable to parse tag \"%s\", skipping", tag)
		}
	}

	// print trace id to stdout so it can be used for next span if desired
	fmt.Printf(span.Context().(jaeger.SpanContext).String())
}

func runCommand(ctx context.Context, span opentracing.Span, name string, arg ...string) error {
	span.SetTag("cmd.cmd", name)
	span.SetTag("cmd.args", strings.Join(arg, " "))
	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		switch e := err.(type) {
		case *exec.ExitError:
			span.SetTag("error", true)
			span.SetTag("exit_code", e.ExitCode())
		default:
			return err
		}
	} else {
		span.SetTag("exit_code", 0)
	}
	return nil
}
