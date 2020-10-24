# Pfiel

Pfiel is a simple tool to report an [opentracing](http://opentracing.io) span to
a [jaeger](https://www.jaegertracing.io) server from the command line, useful
for tracing shell scripts or external processes that can't be directly
instrumented. 

If this is a child span, the extracted trace ID (`uber-trace-id`) should be in
environment variable `TRACE_ID`.

The start time to report for the span will default to now, unless `TRACE_START`
is defined in Unix `date` format.

## Flags

* `-h` command help
* `-v` verbose logging (default: `false`)
* `-y` always sample the span (equivalent to `JAEGER_SAMPLER_TYPE=const` and
  `JAEGER_SAMPLER_PARAM=1`, default: `false`)
* `-n` don't actually send the span to jaeger (overrides `-y`, sets
  `JAEGER_SAMPLER_PARAM=0`, default: `false`)
* `-service string` service name (default: `pfiel`)
* `-op string` operation name (default: `verfolgen`) 

## Environment

* `TRACE_ID` extracted `uber-trace-id` (see below for example extraction)
* `TRACE_START` optional start time for trace in Unix `date` format (e.g. `Thu
  Oct 22 16:06:24 EDT 2020`)

## Example usage

``` sh
export JAEGER_ENDPOINT="http://my-jaeger-collector:14268/api/traces"
export JAEGER_SERVICE_NAME="my_script"
export TRACE_ID="7d7e22c6f96e391:b3185835b0e579c7:0:1" # we extracted this from some parent process

export TRACE_START=`date`
command_1
pfiel -y -op command_1
export TRACE_START=`date`
command_2
pfiel -v -y -op command_2 foo=bar
2020/10/22 16:13:19 pfiel: found TRACE_ID 7d7e22c6f96e391:b3185835b0e579c7:0:1
2020/10/22 16:13:19 pfiel: found TRACE_START Thu Oct 22 16:06:24 EDT 2020
2020/10/22 16:13:19 pfiel: started span 07d7e22c6f96e391:7f6e9f9bd9baf554:b3185835b0e579c7:1
2020/10/22 16:13:19 pfiel: done
```

## Extracting a trace id

Example in Python:

``` python
import jaeger_client
config={
    'sampler': {
        'type': 'const',
        'param': 1,
    },
}
t=jaeger_client.Config(config=config,service_name='test').initialize_tracer()
span=t.start_span('parent')
data={}
t.inject(span.context, 'text_map', data)
# data = {'uber-trace-id': '7d7e22c6f96e391:b3185835b0e579c7:0:1'}
print(data['uber-trace-id'])
span.finish()
```
