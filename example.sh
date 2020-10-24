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
       echo "exit_code=$1 error=true"
   else
       echo "exit_code=$1 error=false"
   fi
}

export JAEGER_SERVICE_NAME=$(basename "$0")
unset TRACE_ID TRACE_START # clear any exisitng tracer state
export TRACE_ID=$(./pfeil -v -op init -y args="$*") && export TRACE_START=$(date)
step 1
./pfeil -v -op step1 $(add_tags $?) && export TRACE_START=$(date)
step 2
./pfeil -v -op step2 $(add_tags $?) && export TRACE_START=$(date)
step 3 1
./pfeil -v -op step3 $(add_tags $?)
