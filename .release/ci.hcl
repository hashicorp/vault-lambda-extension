schema = "1"

project "vault-lambda-extension" {
  team = "vault"
  slack {
    notification_channel = "CRF6FFKEW"
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

event "upload-dev" {
  depends = ["build"]
  action "upload-dev" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "upload-dev"
    depends = ["build"]
  }

  notification {
    on = "fail"
  }
}

event "security-scan-binaries" {
  depends = ["upload-dev"]
  action "security-scan-binaries" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "security-scan-binaries"
    config = "security-scan.hcl"
  }

  notification {
    on = "fail"
  }
}

event "sign" {
  depends = ["security-scan-binaries"]
  action "sign" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "sign"
  }

  notification {
    on = "fail"
  }
}

event "verify" {
  depends = ["sign"]
  action "verify" {
    organization = "hashicorp"
    repository = "crt-workflows-common"
    workflow = "verify"
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
