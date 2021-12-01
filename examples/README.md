# Examples

These examples demonstrate how common patterns can be implemented in Cartographer.
The examples use open source tools to accomplish each step.

The examples have overlapping concerns and therefore use a shared set of templates.
They similarly have shared tool setup (secrets, etc).

- [Source => Image => App](./source-kpack-knative): demonstrates how
  one can set up a `ClusterSupplyChain` that monitors source code updates, creates
  a new image for each update and deploys that image as a running application
  in the cluster.
- [Source => Test => Image => App](./source-tekton-kpack-knative): expands upon
  the example above by adding testing.