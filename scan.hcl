binary {
  go_modules = true
  go_stdlib  = true
  nvd        = false
  oss_index  = true
  osv        = true
  secrets    = false

  triage {
    suppress {
      vulnerabilites = [
        "GHSA-f5pg-7wfw-84q9", # AWS S3 Crypto SDK vuln https://osv.dev/vulnerability/GO-2022-0646
        "GO-2022-0646", # alias
        "GHSA-7f33-f4f5-xwgw", # AWS S3 Crypto SDK vuln https://osv.dev/vulnerability/GO-2022-0635
        "GO-2022-0635" #alias
      ]
    }
  }
}
