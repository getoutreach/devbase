# RFCs

## Table of Contents

<!-- toc -->
- [What is an RFC and when should I create one?](#what-is-an-rfc-and-when-should-i-create-one)
- [Lifecycle of an RFC](#lifecycle-of-an-rfc)
  - [Draft](#draft)
  - [Proposed](#proposed)
  - [Accepted](#accepted)
  - [Rejected](#rejected)
  - [Completed](#completed)
- [How To](#how-to)
  - [Create a new RFC](#create-a-new-rfc)
  - [Update an RFC](#update-an-rfc)
  - [Complete an RFC](#complete-an-rfc)
<!-- /toc -->

This directory contains the "RFCs" for the `devbase` project. An RFC is a "Request for Comments". These are documents that describe the design and implementation of features in the `devbase` project.

## What is an RFC and when should I create one?

An RFC is a document that describes a design and implementation of a feature or large change in the `devbase` project. It is intended to be a living document, and is updated as the scope of the feature/fix evolves over time.

An RFC is created when a feature or fix is deemed to be large enough to warrant a more detailed discussion. This is to ensure that the design and implementation of the feature/fix is well thought out, documented, and given time for feedback from the community.

## Lifecycle of an RFC

### Draft

The "Draft" status is used when an RFC is in the process of being drafted. This is the initial state of an RFC, and is in the form of a "draft" pull request.

### Proposed

A RFC is "Proposed" when it is ready for review. This is the state of an RFC when it is ready for review by the community. The author of the RFC should be prepared to answer questions and make changes to the RFC as needed.

### Accepted

Once an RFC is merged, it is considered "accepted" and may be implemented with the goal of eventual inclusion into the `devbase` project. Being "accepted" is not a rubber stamp, and in particular still does not mean the feature will ultimately be merged; it does mean that in principle all the major stakeholders have agreed to the feature and are amenable to merging it.

An RFC will be updated as development progresses, as it's considered to be a living document that will be updated as the feature evolves. The author, or implementor, of the RFC should update it as needed.

If, during the development of an rfc, the implementation changes significantly, the author should update the RFC to reflect the changes. This will help keep the RFC up to date, and will help the community to understand the changes. If the changes void the original RFC, the author should create a new RFC to supersede the original RFC and note that accordingly.

### Rejected

An RFC can also be "Rejected". The possible reasons for rejection include duplication of effort, being technically unsound, not providing proper motivation or addressing backwards compatibility, or not in keeping with the goals and direction of the `devbase` project.

### Completed

Once an RFC is implemented, it is considered "Completed". This means that the RFC is no longer a living document, and the feature is considered to be complete. The RFC will reflect that it is completed, and contain information about how the process went, and any lessons learned.

## How To

### Create a new RFC

  1. Copy the `0-template.md` file to `rfcs/0000-my-feature.md` (where "my-feature" is a hyphenated version of the title. Don't assign an RFC number yet).
  2. Fill in the RFC. Put care into the details: RFCs that do not; present convincing motivation, demonstrate understanding of the impact of the design, or are disingenuous about the drawbacks or alternatives tend to be poorly-received. You might want to create a [Draft Pull Request](https://github.blog/2019-02-14-introducing-draft-pull-requests/) to make this process easier and more effective.
  3. Submit a pull request. As a pull request the RFC will receive design feedback from the larger community, and the author should be prepared to revise it in response.


### Update an RFC

Updating an RFC follows the same process as creating a new RFC. The author should create a PR updating the current document, as it is considered "living", and pull request the changes into the RFC.

### Complete an RFC

Once an RFC has been "completed" (the functionality has been implemented), the same process as updating an RFC should be followed. The author should create a PR updating the current document and pull request the changes into the RFC. The RFC should be updated to reflect that it is completed, and contain information about how the process went, and any lessons learned.

This is also the opportunity to fine-tune the RFC process itself. If there are changes that could be made to the RFC process, consider creating a new RFC to propose those changes to the RFC process itself.
