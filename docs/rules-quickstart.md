Konveyor - Rule Writing Quickstart Guide
---

Konveyor is an application modernization platform that helps to modernize and migrate applications to new technologies.
This is done through the application of written rules: at its heart, Konveyor is a rules engine.

Konveyor comes packed by default with a set of pre-written rules for several different migration paths,
but users are allowed and encouraged to write their own rules, which can be easily input before analysis. Ideally,
users would contribute their own rules to the Konveyor Community, improving and expanding the number of available
migration paths over time.

1. [What is Kantra?](#what-is-kantra)
   1. [How to install](#how-to-install-kantra)
   2. [How to use](#how-to-use-kantra)
   3. [Usage examples](#usage-examples)
2. [What is a rule?](#what-is-a-rule)

### What is Kantra?
Kantra is a CLI wrapper for the rules engine that powers Konveyor. Kantra allows users to do quick static analysis
without the need to install the full Konveyor platform in a Kubernetes environment. The result is a HTML report that mimics
that of Konveyor.

#### How to install Kantra
- Installing kantra is easy, just go to the [Kantra releases](https://github.com/konveyor/kantra/releases) page and download
a binary for your OS and architecture.
- Additionally, you will need a container platform installed in your system, either podman or docker.

#### How to use Kantra
- Kantra comes pre-loaded with several rulesets that provide different migration paths by default. These available paths
  (also named _targets_ in the Konveyor ecosystem) can be checked out by running `kantra analyze --list-targets`. Specifying
  a migration path is desirable to have a targeted analysis.
- Additionally, more rules can be specified with the `--rules` option.
- When analyzing an application, it is possible to also analyze the code of its dependencies too:
  - `--mode` allows choosing between analyzing only the source code (`source-only`) or dependencies too (`full`, the default)
  - `--analyze-known-libraries` tells the engine to also analyze dependencies that are open source, and therefore whose code is
  generally accessible. This option only makes sense when using `--mode full`.
- When analyzing Java code, both source code and binaries can be analyzed. This can be specified simply by the `--input` option.

#### Usage examples
- Generate report to migrate source code application to EAP8 and OpenJDK21, save the report somewhere else:
  - `kantra --input ~/Apps/my-application --target eap8 --target openjdk21 --output ~/Reports/my-application.report`
- Generate report to analyze binary application for cloud readiness:
  - `kantra --input ~/Apps/my-war-file.war --target cloud-readiness`
- Generate report to migrate to Quarkus from EAP7, but do not analyze dependencies:
  - `kantra --input ~/Apps/my-eap7-application --source eap7 --target quarkus --mode source-only`

### What is a rule?
A rule in Konveyor is a _formally written specification of a **condition**_. If this condition happens to be
true within the application being analyzed, it will raise an **action** (normally an **issue**) in the generated report, this is, a problem within the code
that will need to be changed or fixed by a developer in order for the migration to succeed.
```
analysis -> condition is true -> issue
```

For instance, if we want to migrate an application from [JakartaEE 8 to 9](https://jakarta.ee/blogs/javax-jakartaee-namespace-ecosystem-progress/),
we will need to change all our namespaces from `javax.*` to `jakarta.*`. In this case, the **condition** would be:
```
if you find any usage of javax.*
```
and the action would be:
```
create an issue with a message telling the user to change from javax.* to jakarta.*
```
In the generated report, the user will find an issue, pointing to the specific part of the code where the condition was raised,
telling exactly what needs to be modified.

#### Parts of a rule
Continuing with the previous example, that rule could be coded as follows:
```yaml
# Metadata
  ruleID: javax-to-jakarta-rule-00001
  labels:
    - konveyor.io/source
    - konveyor.io/target=jakarta-ee9+
    - javaee
# Condition
  when:
    java.referenced:
      location: IMPORT
      pattern: javax.*
# Action
  description: The package 'javax' has been replaced by 'jakarta'.
  message: Replace the `javax.*` import statement with `jakarta.*`
  category:
  links:
    - title: Jakarta EE
      url: https://jakarta.ee/
```
There are three main parts that a rule consists of:
- <ins>Metadata</ins>: a set of fields for identification, categorization, etc.
  - Rule ID: the ID of the rule, which must be unique
  - Labels: a set of tags to categorize the rule in different ways, mostly used for filtering
- <ins>Condition (_when_)</ins>: the condition that needs to be true for the rule to be triggered. There are different types of available conditions,
and different language providers can have their own specific conditions unique to the language.
- <ins>Action</ins>: one or more fields that describe the actions to be taken if the rule is triggered:
  - Description: a short title for the issue
  - Message: a longer and more explanatory text describing the problem that needs solving
  - Category: the type of issue, indicating whether the raised issue must be modified (_mandatory_), is _optional_ (ie, for deprecated APIs), or _potential_ (can't be decided)
  - Effort: an approximate number of points indicating the effort needed to do the modification in the code
  - Links: a set of links with more information about the issue

There are some additional fields that will be explained in the following sections.

#### Conditions
Conditions are the most important part of a rule, since they indicate what needs to happen in the code
for an issue to be raised. The different types of conditions are provided by the **language providers**. Since Konveyor can be extended
to be able to analyze any language, and each language has its own quirks and specifications, each language
provider can expose its own conditions (in the form of _capabilities_). In the case of the Java language provider, it exposes two types of conditions:
- `java.referenced`: matches against code references, like a match against a method or an annotated field
- `java.dependency`: matches against the existence of a given dependency of the application being analyzed

An exception to this is **the builtin provider**, which is a generic, built-in provider for creating conditions
such as plain text regex matching or Xpath matching:
- `builtin.filecontent`: matches a given regex on plaintext
- `builtin.file`: matches on a given filename
- `builtin.xml`: matches on an Xpath expression

Conditions can be combined using the logical operators `and`, `or` and `not`:
```yaml
when:
  and:
    - java.referenced:
        location: ANNOTATION
        pattern: org.konveyor.ExampleAnnotation
    - or
      - builtin.filecontent:
          filePattern: ASpecificClass.java
          pattern: some.*regex
      - builtin.file:
          pattern: "^.*\\.properties$"
```

#### Writing rules
The best way to write rules is to check the examples we have in the [default ruleset]()