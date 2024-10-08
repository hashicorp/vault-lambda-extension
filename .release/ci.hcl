# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

schema = "1"

project "vault-lambda-extension" {
  team = "vault"
  slack {
    notification_channel = "C03RXFX5M4L" // #feed-vault-releases
  }
  github {
    organization = "hashicorp"
    repository = "vault-lambda-extension"
    release_branches = ["main"]
  }
}

event "merge" {
  // "entrypoint" to use if build is not run automatically
  // i.e. send "merge" complete signal to orchestrator to trigger build
}

event "build" {
  depends = ["merge"]
  action "build" {
    organization = "hashicorp"
    repository = "vault-lambda-extension"
    workflow = "build"
  }
}

event "prepare" {
  depends = ["build"]
  action "prepare" {
    organization = "hashicorp"
    repository   = "crt-workflows-common"
    workflow     = "prepare"
    depends      = ["build"]
  }

  notification {
    on = "fail"
  }
}

## These are promotion and post-publish events
## they should be added to the end of the file after the verify event stanza.

event "trigger-staging" {
// This event is dispatched by the bob trigger-promotion command
// and is required - do not delete.
}

event "promote-staging" {
  depends = ["trigger-staging"]
  action "promote-staging" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "promote-staging"
    config = "release-metadata.hcl"
  }

  notification {
    on = "always"
  }
}

event "promote-layer-staging" {
  depends = ["promote-staging"]
  action "promote-layer-staging" {
    organization = "hashicorp"
    repository = "vault-lambda-extension-release"
    workflow = "promote-layer-staging"
  }
}

event "trigger-production" {
// This event is dispatched by the bob trigger-promotion command
// and is required - do not delete.
}

event "promote-production" {
  depends = ["trigger-production"]
  action "promote-production" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "promote-production"
  }

  notification {
    on = "always"
  }
}

event "promote-layer-production" {
  depends = ["promote-production"]
  action "promote-layer-production" {
    organization = "hashicorp"
    repository = "vault-lambda-extension-release"
    workflow = "promote-layer-production"
  }
}
