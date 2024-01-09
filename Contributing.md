# Contributing 

- [Contributing](#contributing)
	- [Code of Conduct](#code-of-conduct)
	- [Reporting security issues](#reporting-security-issues)
	- [Reporting general issues](#reporting-general-issues)
	- [Code and doc contribution](#code-and-doc-contribution)
		- [Workspace Preparation](#workspace-preparation)
		- [Branch Definition](#branch-definition)
		- [Code Review](#code-review)
		- [PR Description](#pr-description)
	- [Engage to help anything](#engage-to-help-anything)


## Code of Conduct

Please do check our [Code of Conduct](CODE_OF_CONDUCT.md) before making contributions.

## Reporting security issues

We take security issues seriously and discourage anyone to spread security issues. If you find a security issue in KusionStack Rollout, please do not discuss it in public and even do not open a public issue. Instead we encourage you to send us a private email to kusionstack@gmail.com to report the security issue.

## Reporting general issues

Any user is welcome be a contributor. If you have any feedback for the project, feel free to open an issue. 

Since KusionStack Rollout development will be collaborated in a distributed manner, we appreciate **WELL-WRITTEN**, **DETAILED**, **EXPLICIT** issue reports. To make communication more efficient, we suggest everyone to search if your issue is an existing one before filing a new issue. If you find it to be existing, please append your details in the issue comments.

There are lot of cases for which you could open an issue:

* Bug report
* Feature request
* Performance issues
* Feature proposal
* Feature design
* Help wanted
* Doc incomplete
* Test improvement
* Any questions about the project, and so on

Please remind that when filing a new issue, do remove the sensitive data from your post. Sensitive data could be password, secret key, network locations, private business data and so on.

## Code and doc contribution

Any action that may make KusionStack Rollout better is encouraged. The action can be realized via a PR (short for pull request).

* If you find a typo, try to fix it!
* If you find a bug, try to fix it!
* If you find some redundant codes, try to remove them!
* If you find some test cases missing, try to add them!
* If you could enhance a feature, please **DO NOT** hesitate!
* If you find code implicit, try to add comments to make it clear!
* If you find tech debts, try to refactor them!
* If you find document incorrect, please fix that!

It is impossible to list them completely, we are looking forward to your pull requests.
Before submitting a PR, we suggest you could take a look at the PR rules here.

* [Workspace Preparation](#workspace-preparation)
* [Branch Definition](#branch-definition)
* [Commit Rules](#commit-rules)
* [PR Description](#pr-description)

### Workspace Preparation

We assume you have a GitHub ID already, then you could finish the preparation in the following steps:

1. **FORK** KusionStack Rollout to your repository. To make this work, you just need to click the button `Fork` in top-right corner of [rollout](https://github.com/KusionStack/rollout) main page. Then you will end up with your repository in `https://github.com/<username>/rollout`, in which `username` is your GitHub ID.
1. **CLONE** your own repository to develop locally. Use `git clone https://github.com/<username>/rollout.git` to clone repository to your local machine. Then you can create new branches to finish the change you wish to make.
1. **Set Remote** upstream to be rollout using the following two commands:

```
git remote add upstream https://github.com/KusionStack/rollout.git
git remote set-url --push upstream no-pushing
```

With this remote setting, you can check your git remote configuration like this:

```
$ git remote -v
origin     https://github.com/<username>/rollout.git (fetch)
origin     https://github.com/<username>/rollout.git (push)
upstream   https://github.com/KusionStack/rollout.git (fetch)
upstream   no-pushing (push)
```

With above, we can easily synchronize local branches with upstream branches.

### Branch Definition

Right now we assume every contribution via pull request is for the `main` branch in KusionStack Rollout.
There are several other branches such as rc branches, release branches and backport branches.
Before officially releasing a version, we may checkout a rc (release candidate) branch for more testings.
When officially releasing a version, there may be a release branch before tagging which will be deleted after tagging.
When backporting some fixes to existing released version, we will checkout backport branches.


### Code Review


For a brief description of the importance of code review, please read [On Code Review](/contributors/guide/expectations.md#code-review).  
There are two aspects of code review: giving and receiving.

To make it easier for your PR to receive reviews, consider the reviewers will need you to:

* Follow the project [coding conventions]([coding-conventions.md](https://github.com/kubernetes/community/blob/master/contributors/guide/coding-conventions.md))
* Write [good commit messages](https://chris.beams.io/posts/git-commit/)
* Break large changes into a logical series of smaller patches which individually make easily understandable changes, and in aggregate solve a broader issue

Reviewers, the people giving the review, are highly encouraged to revisit the [Code of Conduct](./CODE_OF_CONDUCT.md).

### PR Description

PR is the only way to make change to this project. To help reviewers, we actually encourage contributors to make PR description as detailed as possible.

## Engage to help anything

GitHub is the primary place for contributors to collaborate. Although contributions via PR is an explicit way to help, we still call for any other types of helps.

* Reply to other's issues if you could;
* Help solve other user's problems;
* Help review other's PR design;
* Help review other's codes in PR;
* Discuss to make things clearer;
* Advocate KusionStack Rollout technology beyond GitHub;
* Write blogs on KusionStack Rollout, and so on.

In a word, **ANY HELP CAN BE A CONTRIBUTION.**
