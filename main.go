package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	jaeger "github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
)

var (
	version = "0.1.0"
	verbose = flag.Bool("v", false, "enable verbose/debug logging (default false)")
	sample  = flag.Bool("y", false, "always sample new trace (sets SAMPLER_TYPE=const, SAMPLER_PARAM=1)")
	noop    = flag.Bool("n", false, "never sample new trace (overrides -y, sets SAMPLER_PARAM=0)")
	service = flag.String("svc", "", "service name for trace, overrides JAEGER_SERVICE_NAME")
	op      = flag.String("op", "verfolgen", "operation name for span")
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `Pfiel version %s
Usage: %s [opts] [tag1=val1] [tag2=val2] ...:

Pfiel is a tool to send an OpenTracing span to a Jaeger agent or collector.
Use the following environment variables to control the span:

  TRACE_ID       uber-trace-id, extracted from parent process
  TRACE_START    timestamp in Unix date format to record for start of span

Arguments will be parsed into key=value pairs and added as tags.

Use the following environment variables to configure Jaeger: (common settings,
see https://github.com/jaegertracing/jaeger-client-go for the full list).

  JAEGER_SERVICE_NAME      The service name (overriden by -svc).
  AEGER_AGENT_HOST         The hostname for communicating with agent via UDP
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

func main() {
	flag.Parse()

	l := log.New(os.Stderr, "pfiel: ", log.LstdFlags|log.Lmsgprefix)
	vl := log.New(os.Stderr, "pfiel: ", log.LstdFlags|log.Lmsgprefix)
	if !*verbose {
		vl.SetOutput(ioutil.Discard)
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

	span := tracer.StartSpan(*op, opts...)
	vl.Printf("started span %s", span)
	defer span.Finish()

	// parse tags from arguments
	for _, arg := range flag.Args() {
		kv := strings.SplitN(arg, "=", 2)
		if len(kv) == 2 {
			vl.Printf("setting tag %s to %s", kv[0], kv[1])
			span.SetTag(kv[0], kv[1])
		} else {
			l.Printf("unable to parse tag argument %s, skipping", arg)
		}
	}
	vl.Printf("done")
}
