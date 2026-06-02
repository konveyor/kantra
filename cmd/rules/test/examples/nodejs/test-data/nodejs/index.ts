/**
 * Sample sources for nodejs provider tests (rules node-sample-rule-001/002).
 * Avoid the substring "log" here so node-sample-rule-003 stays unmatched (see rule description).
 */
export class Greeter {
  private name: string;

  constructor(name: string) {
    this.name = name;
  }

  hello(): string {
    return `Hello, ${this.name}`;
  }
}

// Lowercase `greeter` identifier so node-sample-rule-001 (`pattern: greeter`) matches;
// the class name `Greeter` alone did not match (pattern is case-sensitive).
export const greeter = new Greeter("demo");
