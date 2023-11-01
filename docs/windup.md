## Known differences between Windup and kantra output

#### `--rules` + `--target`
- In Windup, you can specify a custom rule with a target. If the custom rule does not
have a label with the given target specified, it will **still** run the rule.  

- In kantra, if a rule is given as well as a target, but the given rule **does not**
have the target label, the given rule will not match. 
    - You must add the target label to the custom rule (if applicable) in
    order to run this rule.
