Konveyor - Rule Writing Quickstart Guide
---

Konveyor is an application modernization platform that helps to modernize and migrate applications to new technologies.
This is done through the application of written rules: at its heart, Konveyor is a rules engine.

Konveyor comes packed by default with a set of pre-written rules for several different migration paths,
but users are allowed and encouraged to write their own rules, which can be easily input before analysis. Ideally,
users would contribute their own rules to the Konveyor Community, improving and expanding the number of available
migration paths over time.

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
telling exactly what needs to be modified. We will further explain what each field means.

### Parts of a rule
Continuing with the previous example, the rule could be coded as follows:
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
  - Category: the type of issue, indicating whether the raised issue must be modified (mandatory), is optional (ie, for deprecated APIs), or potential (can't be decided)
  - Effort: an approximate number of points indicating the effort needed to do the modification in the code
  - Links: a set of links with more information and potentially deeper explanations about the issue

There are some additional fields that will be explained in the following sections.

### Conditions
Conditions are the most important part of a rule, since they indicate 