# RFCs

## Table of Contents

<!-- toc -->
- [What is an RFC and when should I create one?](#what-is-an-rfc-and-when-should-i-create-one)
- [Creating a new RFC](#creating-a-new-rfc)
- [Lifecycle of an RFC](#lifecycle-of-an-rfc)
<!-- /toc -->

This directory contains the "RFCs" for the `devbase` project. An RFC is a "Request for Comments". These are documents that describe the design and implementation of features in the `devbase` project.

## What is an RFC and when should I create one?

An RFC is a document that describes a design and implementation of a feature in the `devbase` project. It is intended to be a living document, and is updated as the scope of the feature/fix evolves over time.

An RFC is created when a feature or fix is deemed to be large enough to warrant a more detailed discussion. This is to ensure that the design and implementation of the feature/fix is well thought out, documented, and given time for feedback from the community.

## Creating a new RFC

  1. Copy the `0-template.md` file to `rfcs/0000-my-feature.md` (where "my-feature" is a hyphenated version of the title. Don't assign an RFC number yet).
  2. Fill in the RFC. Put care into the details: RFCs that do not; present convincing motivation, demonstrate understanding of the impact of the design, or are disingenuous about the drawbacks or alternatives tend to be poorly-received. You might want to create a [Draft Pull Request](https://github.blog/2019-02-14-introducing-draft-pull-requests/) to make this process easier and more effective.
  3. Submit a pull request. As a pull request the RFC will receive design feedback from the larger community, and the author should be prepared to revise it in response.

## Lifecycle of an RFC

Once an RFC is merged, it is considered "active" and may be implemented with the goal of eventual inclusion into the `devbase` project. Being "active" is not a rubber stamp, and in particular still does not mean the feature will ultimately be merged; it does mean that in principle all the major stakeholders have agreed to the feature and are amenable to merging it.

An RFC will be updated as development progresses, as it's considered to be a living document that will be updated as the feature evolves. The author, or implementor, of the RFC should update it as needed.

If, during the development of an rfc, the implementation changes significantly, the author should update the RFC to reflect the changes. This will help keep the RFC up to date, and will help the community to understand the changes. If the changes void the original RFC, the author should create a new RFC to supersede the original RFC and note that accordingly.
