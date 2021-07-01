# Copyright 2016 the original author or authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# -------------
# Build stage

FROM golang:1.16.3-alpine3.13 AS build-env

# Install required packages
RUN apk add --no-cache build-base=0.5-r2

# Prepare directory structure
WORKDIR /go/src/github.com/opencord/openolt-scale-tester
COPY . .

# Build openolt
SHELL ["/bin/ash", "-o", "pipefail", "-c"]
ENV GO111MODULE=on
RUN  go build -mod=vendor -o /go/bin/openolt-scale-tester

# -------------
# Image creation stage

FROM alpine:3.13.6

# Set the working directory
WORKDIR /app

# Copy required files
COPY --from=build-env /go/bin/openolt-scale-tester /app/
COPY --from=build-env /go/src/github.com/opencord/openolt-scale-tester/tech-profiles /app/
