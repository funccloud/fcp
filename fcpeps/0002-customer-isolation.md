---
title: Customer Isolation
authors:
  - "@felipeweb"
creation-date: 2025-03-25
status: proposed
---

# FCPEP-0002: Customer Isolation

## Table of Contents

<!-- toc -->

- [Summary](#summary)
- [Motivation](#motivation)
<!-- /toc -->

## Summary

Adds the ability to isolate different client applications running on the platform by creating a new `Workspaces` API (CRD). A workspace can be of type `personal` or `organization`, where the workspace of type personal belongs to a user of the platform, while the one of type organization belongs to an organization that can have one or more users of the platform as its owner.

## Motivation
