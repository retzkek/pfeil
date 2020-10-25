#!/bin/bash

step() {
   echo "doing step $1"
   sleep 1
   if [ -z "$2" ]; then
       return 0
   else
       return "$2"
   fi
}

add_tags() {
   if [ "$1" != "0" ]; then
       echo "exit_code=$1,error=true"
   else
       echo "exit_code=$1,error=false"
   fi
}

export JAEGER_SERVICE_NAME=$(basename "$0")
unset TRACE_ID TRACE_START # clear any exisitng tracer state
echo ">>> start a new trace, and use the trace id as parent for following spans"
export TRACE_ID=$(./pfeil -v -y -t args="$*" init) && export TRACE_START=$(date)
echo ">>> do some work"
step 1
./pfeil -v -t "$(add_tags $?)" step1 >/dev/null && export TRACE_START=$(date)
echo ">>> do some more work"
step 2
./pfeil -v -t "$(add_tags $?)" step2 >/dev/null && export TRACE_START=$(date)
echo ">>> maybe too much work, this will fail!"
step 3 1
./pfeil -v -t "$(add_tags $?)" step3 >/dev/null && export TRACE_START=$(date)
echo ">>> or we can run a command in a subprocess and automatically capture the exit code"
./pfeil -v step4 sleep 1 > /dev/null && export TRACE_START=$(date)
echo ">>> even something more complicated, that also fails"
# note the -- to prevent pfeil from trying to handle the -c flag itself
./pfeil -v step5 -- /bin/bash -c 'echo "doing step 5" && sleep 1 && false'
