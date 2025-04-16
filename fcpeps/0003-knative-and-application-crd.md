---
title: Adoption of Knative and Application CRD
authors:
  - "@felipeweb"
creation-date: 2025-04-16
status: proposed
last-updated: 2025-04-16
---

# FCPEP-0003: Adoption of Knative and Application CRD

## Summary

This FCPEP proposes the adoption of Knative for serverless workload management within the project. Knative provides a robust and feature-rich platform for deploying and managing serverless applications, and integrating it will allow us to leverage existing solutions rather than reinventing the wheel. To simplify Knative configuration and improve user experience, we also propose the creation of an Application Custom Resource Definition (CRD), including support for custom domains.

## Motivation

Currently, deploying and managing serverless workloads within the project would require significant development effort to implement features such as autoscaling, traffic management, and revisioning. Knative offers these functionalities out-of-the-box, allowing us to focus on core project logic instead of infrastructure management.

However, Knative's native configuration can be complex, requiring users to define multiple resources (e.g., Service, Route, Configuration). This complexity can be a barrier to entry for new users and increase the potential for errors. An Application CRD would abstract away this complexity, providing a simplified and more user-friendly interface for deploying and managing applications.

## Goals

* Integrate Knative Serving for serverless workload management.
* Create an Application CRD to simplify Knative configuration.
* Provide a seamless and intuitive experience for deploying and managing serverless applications.

## Proposal

### Knative Integration

We will integrate Knative Serving into the project, leveraging its core capabilities, such as:

*   **Autoscaling:** Dynamically scale applications based on incoming traffic.
*   **Traffic Management:** Control traffic flow between different application revisions.
*   **Revisioning:** Easily manage and roll back to previous application versions.
*   **Routing:** Configure how incoming requests are routed to specific application instances.
*   **Certificate Management:** Automatically provision and manage TLS certificates for Knative services.
*   **Build Integration:** Utilize Knative Build or Tekton Pipelines for building container images from source code. (Further FCPEP may be needed to detail build process integration)

This integration will involve:

1. Installing Knative Serving on the target Kubernetes cluster.
2. Configuring the project to utilize Knative APIs for deploying and managing applications.
3. Potentially, contributing to Knative project to address any specific needs or gaps identified during integration.

### Application CRD

We will define an Application CRD that encapsulates the necessary Knative resources for deploying an application. This CRD will provide a single, unified resource for users to interact with, simplifying the configuration process and supporting custom domain assignment via a `domain` field.

The Application CRD will include fields for:

* **Image:** The container image to deploy.
* **Scale:** Minimum and maximum number of replicas.
* **Resources:** CPU and memory limits and requests.
* **Environment Variables:** Configuration settings for the application.
* **Traffic Splits:** Distribution of traffic between different revisions.
* **Custom Domain:** Optionally specify a custom domain for the application using the `domain` field.
* **Optional Addons/Features:**  e.g.,  Automatic TLS,  Integration with Service Mesh (if applicable)

Example Application CRD manifest:
```
yaml
apiVersion: workload.fcp.funccloud.com/v1alpha1
kind: Application
metadata:
  name: my-app
  namespace: <must be equal spec.workspace>
spec:
  workspace: <workspace name>
  image: gcr.io/my-project/my-app:latest
  scale:
    minReplicas: 1
    maxReplicas: 10
  resources:
    limits:
      cpu: "100m"
      memory: "128Mi"
    requests:
      cpu: "50m"
      memory: "64Mi"
  env:
    - name: MY_VAR
      value: "my-value"
  traffic:
    - revisionName: my-app-v1
      percent: 100
  # Example of an optional feature
  enableTLS: true
  # Example of custom domain support
  domain: "app.example.com"
```
A controller will be implemented to reconcile Application CRD instances, translating them into the corresponding Knative resources (Service, Route, Configuration) and ensuring the desired state is maintained, including custom domain configuration.

### User Experience

Users will interact with the system primarily through the Application CRD, creating and managing applications using a single, intuitive resource.  This will significantly reduce the learning curve and the potential for misconfigurations compared to directly interacting with Knative's native resources.

## Alternatives Considered

* **Building a custom serverless platform from scratch:** This option was rejected due to the significant development effort and the risk of reinventing existing solutions.
* **Using a different serverless platform:** Other platforms were evaluated, but Knative was chosen due to its open-source nature, Kubernetes-native design, and strong community support.

## Risks and Mitigation

* **Knative integration complexity:** Thorough testing and documentation will be crucial to ensure a smooth integration.
* **Application CRD design challenges:** Careful design and user feedback will be incorporated to ensure the CRD effectively simplifies configuration without sacrificing flexibility.
* **Potential conflicts with existing Knative installations:**  The integration process will need to account for existing Knative deployments and avoid conflicts. Namespacing and clear documentation will help mitigate this.
* **Learning curve for developers new to Knative:**  Comprehensive documentation and training materials will be provided to support developers transitioning to the new platform.

## Graduation Criteria

* Successful integration of Knative Serving into the project.
* Functional and well-tested Application CRD and controller.
* Comprehensive documentation for users on how to use the Application CRD and manage their applications.
* Demonstrable improvements in developer productivity and application management efficiency.

## Implementation Plan

1. **Prototype:**  Create a basic prototype integrating Knative and a rudimentary Application CRD.
2. **Testing and Refinement:**  Thoroughly test the prototype and refine the Application CRD based on feedback.
3. **Controller Development:** Implement the controller to reconcile Application CRD instances.
4. **Documentation:**  Create comprehensive documentation for users and developers.
5. **Integration and Deployment:** Integrate the solution into the project and deploy it to the target environment.
6. **Iteration and Improvement:** Continuously iterate on the design and implementation based on user feedback and evolving project needs.

## Open Questions

* Specific details of build integration (Knative Buildpacks, Tekton Pipelines, or other solutions) need to be defined in a separate FCPEP.
* Specific features and capabilities of supporting tools (CLI, UI) need to be defined.

## Conclusion

Adopting Knative and introducing an Application CRD will significantly enhance our ability to manage serverless workloads, reduce development effort, and improve the user experience. This proposal outlines a path towards a more efficient and scalable platform for deploying and managing applications within the project.