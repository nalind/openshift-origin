#!/bin/sh

set -euo pipefail

# Note that in this case we're looking for a config map.  Those get mounted by
# name under /var/run/configs/openshift.io/build, with the associated
# DestinationDir values from the Build object encoded in $BUILD.
# A Docker or Source builder would clone the git repository named in the
# $SOURCE_REPOSITORY env variable, copy the config map contents into a
# subdirectory of the cloned source tree (or a subdirectory of it), and use the
# result as the source for the build.  We could dig the location set in the
# Build object out of $BUILD and copy the contents there, but for this test
# that doesn't tell us anything we didn't already know or check, so just look
# at the location where the build controller mounted the config map and call it
# done.
cd /var/run/configs/openshift.io/build/custom-configmap

# OUTPUT_REGISTRY and OUTPUT_IMAGE are env variables provided by the custom
# build framework
TAG="${OUTPUT_REGISTRY}/${OUTPUT_IMAGE}"

cp -R /var/run/configs/openshift.io/certs/certs.d/* /etc/containers/certs.d/

# buildah requires a slight modification to the push secret provided by the service account in order to use it for pushing the image
echo "{ \"auths\": $(cat /var/run/secrets/openshift.io/pull/.dockercfg)}" > /tmp/.pull
echo "{ \"auths\": $(cat /var/run/secrets/openshift.io/push/.dockercfg)}" > /tmp/.push

# performs the build of the new image defined by Dockerfile.sample
buildah --authfile /tmp/.pull --storage-driver vfs bud --isolation chroot -t ${TAG} .
# push the new image to the target for the build
buildah --authfile /tmp/.push --storage-driver vfs push ${TAG}
