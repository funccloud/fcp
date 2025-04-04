---
title: Customer Isolation
authors:
  - "@felipeweb"
creation-date: 2025-03-25
last-updated: 2025-03-30
status: implemented
---

# FCPEP-0002: Customer Isolation

## Table of Contents

<!-- toc -->

- [Summary](#summary)
- [Motivation](#motivation)
- [Goals](#goals)
- [Non-Goals](#non-goals)
- [Requirements](#requirements)
- [Proposal](#proposal)
<!-- /toc -->

## Summary

Adds the ability to isolate different client applications running on the platform by creating a new `Workspaces` API (CRD). A workspace can be of type `personal` or `organization`, where the workspace of type personal belongs to a user of the platform, while the one of type organization belongs to an organization that can have one or more users of the platform as its owner.

## Motivation

The FCP platform is designed to host and manage applications for a diverse range of users and organizations. As the platform grows, it becomes increasingly important to ensure that applications and resources belonging to different clients are properly isolated from each other. Without a robust isolation mechanism, several critical issues can arise:

- **Security Vulnerabilities:** In the absence of isolation, a vulnerability or misconfiguration in one user's application could potentially compromise the security of other users' applications or data. This poses a significant risk to the confidentiality and integrity of sensitive information.
- **Resource Conflicts:** Multiple applications running on the same platform without isolation may compete for shared resources such as CPU, memory, and network bandwidth. This can lead to performance degradation, instability, and unpredictable behavior for all users.
- **Data Integrity Concerns:** Without proper isolation, there's a risk of accidental or malicious data access or modification between different users' applications.
- **Operational Overhead:** Managing and troubleshooting issues in a shared environment without clear boundaries can be complex and time-consuming.
- **Compliance Requirements:** Many organizations have strict compliance requirements that mandate data and resource isolation. Without this capability, the FCP platform may not be suitable for certain use cases.

To address these challenges, this FCPEP proposes the introduction of the `Workspaces` API. This new API will provide a fundamental building block for isolating client applications and their associated resources. By creating distinct workspaces, we can:

- **Enhance Security:** Prevent unauthorized access and interference between different client applications.
- **Improve Resource Management:** Ensure fair resource allocation and prevent resource contention.
- **Simplify Administration:** Provide clear boundaries for managing and troubleshooting issues.
- **Enable Compliance:** Support organizations with strict isolation requirements.

The `Workspaces` API will support two types of workspaces: `personal` and `organization`. This distinction is essential because:

- **`Personal` Workspaces:** Cater to individual users who need a dedicated space for their applications and data.
- **`Organization` Workspaces:** Enable teams and companies to collaborate and share resources within a controlled environment, with clear ownership and access control.

By implementing this FCPEP, we will significantly enhance the security, stability, and manageability of the FCP platform, making it a more attractive and reliable solution for a wider range of users and organizations.

## Goals

- **Establish Clear Isolation Boundaries:** Define and implement the `Workspaces` API to create distinct boundaries between different client applications and their resources.
- **Enhance Security:** Prevent unauthorized access and interference between applications belonging to different users or organizations.
- **Improve Resource Management:** Ensure that resources are allocated fairly and that applications do not negatively impact each other's performance.
- **Simplify Administration:** Provide a clear and intuitive way to manage resources and troubleshoot issues within isolated workspaces.
- **Support Compliance:** Enable the FCP platform to meet the isolation requirements of organizations with strict compliance needs.
- **Enable Collaboration:** Allow teams and organizations to collaborate effectively within `organization` workspaces.
- **Support Individual Users:** Provide `personal` workspaces for individual users to manage their applications and data.
- **Define the API:** Define the `Workspaces` API (CRD) and its specifications.
- **Define the types:** Define the `personal` and `organization` workspace types.

## Non-Goals

- **Fine-Grained Resource Quotas:** This FCPEP does not aim to implement fine-grained resource quotas or limits within workspaces. Resource management will be addressed at a higher level.
- **Network Isolation:** This FCPEP does not address network-level isolation between workspaces. Network policies and other network-level security measures are out of scope for this proposal.
- **Authentication and Authorization:** This FCPEP does not define the authentication and authorization mechanisms for accessing workspaces. These will be addressed in separate FCPEPs.
- **Detailed Billing and Cost Allocation:** This FCPEP does not define how billing or cost allocation will be handled for workspaces.
- **Multi-tenancy:** This FCPEP does not aim to implement a full multi-tenancy solution. It only aims to implement customer isolation.

## Requirements

- **Personal Workspace Resource Addition:** Only the user who owns a `personal` workspace can add resources to it.
- **Organization Workspace Multiple Owners:** An `organization` workspace can have more than one owner.

* **Personal Workspace Single Owner:** Only one user can be the owner of a `personal` workspace.
* **Personal Workspace Non-Transferable:** A `personal` workspace is non-transferable.
* **Organization Workspace Transferable:** An `organization` workspace is transferable.

## Proposal

To implement customer isolation, FCP will introduce a new `Workspace` Custom Resource Definition (CRD) at the cluster scope. This CRD will be part of the `tenancy.fcp.funccloud.com/v1alpha1` API group.

The `Workspace` CRD will define the boundaries for isolating resources and applications belonging to different users or organizations. Each `Workspace` will be of one of two types: `personal` or `organization`.

**Workspace and Namespace Relationship**

Each `Workspace` will have a one-to-one (1:1) relationship with a Kubernetes Namespace. This means that when a `Workspace` is created, a corresponding Kubernetes Namespace will also be created. Similarly, when a `Workspace` is deleted, its associated Namespace will also be deleted. This ensures that each `Workspace` has a dedicated and isolated environment within the cluster.

**Workspace Ownership and RBAC**

To enforce ownership within each `Workspace`, a set of Kubernetes Role-Based Access Control (RBAC) policies will be automatically created and applied to the corresponding Namespace. These RBAC policies will ensure that:

- Only the designated owner(s) of a `personal` or `organization` `Workspace` have the necessary permissions to manage resources within the associated Namespace.
- Users who are not owners of a `Workspace` will not have access to the resources within its associated Namespace.
- For `organization` workspaces, multiple owners can be defined, and they will all have the necessary permissions to manage resources within the associated Namespace.

**Workspace Owner Permissions**

The owner of a `Workspace` will have full control over the resources within their associated Namespace, with the exception of resources that are considered core to the FCP platform. This means that the owner will be able to:

- **Create** new resources within the Namespace.
- **Edit** existing resources within the Namespace.
- **View** the details of resources within the Namespace.
- **Watch** for changes to resources within the Namespace.
- **Delete** resources within the Namespace.

Here's a basic structure of the `Workspace` CRD:

```yaml
apiVersion: tenancy.fcp.funccloud.com/v1alpha1
kind: Workspace
metadata:
  name: <workspace-name> # The name of the workspace.
spec:
  type: <organization | personal> # The type of the workspace.
  owners:
    - name: <owner-name> # The name of the owner (User or Group).
      kind: <User | Group> # The kind of the owner (User or Group).
status:
  observedGeneration: # Observed Generation of the Workspace.
  conditions: # Represents the observations of a Workspace's current state.
    - type: Ready # Type of condition.
      status: <True | False> # Status of the condition, one of True, False, Unknown.
      reason: <ExampleReason> # The reason for the condition's last transition.
      message: <ExampleMessage> # A human readable message indicating details about the transition.
```
