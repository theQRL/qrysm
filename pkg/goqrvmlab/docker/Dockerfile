#
# The Mega Dockerfile
#
# This dockerfile is an attempt to bundle the following components into 
# one big dockerfile:
#
# - [x] Goqrvmlab binary 'generic-fuzzer'
# - [x] Go-qrl binary 'qrvm'
# - [x] QrvmOne vm binary 'qrvmone'
#

#---------------------------------------------------------------
# golang-builder (debian-based)
#---------------------------------------------------------------
FROM golang:latest as golang-builder 

#
# Go-qrvmlab
#

RUN git clone https://github.com/theQRL/qrysm/pkg/goqrvmlab --depth 1
RUN cd goqrvmlab && \
  go build ./cmd/generic-fuzzer && \
  go build ./cmd/checkslow && \
  go build ./cmd/minimizer && \
  go build ./cmd/repro && \
  go build ./cmd/runtest && \
  go build ./cmd/tracediff && \
  go build ./cmd/traceview

#
# GQRL
#

RUN git clone https://github.com/theQRL/go-qrl --depth 1
RUN cd go-qrl && go run build/ci.go install -static ./cmd/qrvm


#---------------------------------------------------------------
# debian-builder
#---------------------------------------------------------------

#
# QRVMONE 
#
#
# qrvmone requires g++ v13, which is _not_ available in debian bookworm (the golang image)
# but it works with debian:testing (at the time of writing this) 

FROM debian:testing as debian-builder
RUN apt-get update -q && apt-get install -qy --no-install-recommends git make \
    ca-certificates g++ cmake ninja-build libgmp-dev
COPY --from=golang-builder /build.sequential /build.sequential

RUN git clone https://github.com/theQRL/qrvmone.git --depth 1 --recurse-submodules
RUN cd qrvmone && cmake -S . -B build -DQRVMONE_TESTING=ON -DQRVMONE_PRECOMPILES_SILKPRE=1
RUN cd qrvmone && cmake --build build --parallel
RUN cp /qrvmone/build/bin/qrvmone-statetest /qrvmone-statetest
RUN cp /qrvmone/build/lib/libqrvmone.so.0.12 /libqrvmone.so.0.12

#
# Main non-builder
#

FROM debian:testing

RUN apt-get update -q
# nethtest requires libssl-dev
RUN apt-get install -qy --no-install-recommends libssl-dev
# besu requires openjdk-17-jre
RUN apt-get install -qy --no-install-recommends  openjdk-17-jre 

# Go-qrvmlab targets
COPY --from=golang-builder /go/goqrvmlab/generic-fuzzer /
COPY --from=golang-builder /go/goqrvmlab/checkslow  /
COPY --from=golang-builder /go/goqrvmlab/minimizer /
COPY --from=golang-builder /go/goqrvmlab/repro /
COPY --from=golang-builder /go/goqrvmlab/runtest /
COPY --from=golang-builder /go/goqrvmlab/tracediff /
COPY --from=golang-builder /go/goqrvmlab/traceview /
COPY --from=golang-builder /go/goqrvmlab/qrvms/testdata/ /testdata/

COPY --from=golang-builder /go/go-qrl/build/bin/qrvm /gqrlvm
ENV GQRL_BIN=/gqrlvm

COPY --from=debian-builder /qrvmone-statetest /qrvmone
COPY --from=debian-builder /libqrvmone.so.0.12 /lib/libqrvmone.so.0.12
ENV QRVMO_BIN=/qrvmone

ENTRYPOINT ["/generic-fuzzer","--gqrlbatch=/gqrlvm", "--qrvmone=/qrvmone","--fork=Zond"]
