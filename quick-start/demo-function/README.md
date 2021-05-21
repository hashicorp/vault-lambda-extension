# demo-function

This directory contains a demo function for the purposes of testing the
vault-lambda-extension. It attempts to connect and query users from a database
using two different sets of credentials; one set fetched from disk (which the
extension wrote during initialization) and the other set from the proxy server
which is queried during the function runtime.
