#!/bin/sh

set -eu

exec 9>/var/lock/telefeeds-deploy.lock
flock 9

cd /srv/telefeeds/telefeeds.botapi

image_tag="${IMAGE_TAG:-}"
if [ -z "$image_tag" ]; then
  if [ "${PREBUILT_IMAGE:-0}" = "1" ]; then
    echo "IMAGE_TAG is required with PREBUILT_IMAGE=1" >&2
    exit 2
  fi
  build_version="$(git describe --tags --long --always --dirty 2>/dev/null || printf unknown)"
  build_date="$(date -u +'%Y-%m-%d_%H-%M-%S')"
  image_tag="${build_version}-${build_date}"
fi

if [ "${PREBUILT_IMAGE:-0}" != "1" ]; then
  DOCKER_BUILDKIT=1 docker build \
    --tag "telefeeds-botapi:${image_tag}" \
    --tag telefeeds-botapi:latest \
    .
  docker save "telefeeds-botapi:${image_tag}" telefeeds-botapi:latest | k3s ctr images import -
fi

/srv/telefeeds/telefeeds.deploy/bin/deploy-image botapi "$image_tag"
TELEFEEDS_DEPLOY_LOCKED=1 \
  /srv/telefeeds/telefeeds.deploy/bin/prune-build-storage

echo "Deployed telefeeds-botapi:${image_tag}"
