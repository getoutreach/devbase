#!/usr/bin/env bash
# Get bootstrap information

get_app_name() {
  yq -r '.name' <"service.yaml"
}
