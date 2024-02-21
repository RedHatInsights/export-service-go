#!/bin/bash

# --------------------------------------------
# Options that must be configured by app owner
# --------------------------------------------
APP_NAME="export-service"  # name of app-sre "application" folder this component lives in
COMPONENT_NAME="$APP_NAME"  # name of app-sre "resourceTemplate" in deploy.yaml for this component
IMAGE="quay.io/cloudservices/export-service"

IQE_PLUGINS="export_service"
#IQE_IMAGE_TAG="export-service-b4619e78"
#IQE_MARKER_EXPRESSION="smoke"
IQE_FILTER_EXPRESSION=""
IQE_CJI_TIMEOUT="30m"
IQE_ENV="clowder_smoke"
IQE_ENV_VARS="DYNACONF_USER_PROVIDER__rbac_enabled=false"

# Install bonfire repo/initialize
CICD_URL=https://raw.githubusercontent.com/RedHatInsights/bonfire/master/cicd
curl -s $CICD_URL/bootstrap.sh > .cicd_bootstrap.sh && source .cicd_bootstrap.sh

mkdir -p $WORKSPACE/artifacts

source $CICD_ROOT/build.sh
# source $APP_ROOT/unit_test.sh
source "${CICD_ROOT}/deploy_ephemeral_env.sh"
source "${CICD_ROOT}/cji_smoke_test.sh"
source "${CICD_ROOT}/post_test_results.sh"

