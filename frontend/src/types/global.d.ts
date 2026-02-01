// Global type declarations for TypeScript build
// These types should be available globally but may be missing during build

declare global {
  // TypeScript utility types
  type ReturnType<T extends (...args: any) => any> = T extends (...args: any) => infer R ? R : any;
  type Omit<T, K extends keyof any> = Pick<T, Exclude<keyof T, K>>;
  type Partial<T> = { [P in keyof T]?: T[P] };
  type Required<T> = { [P in keyof T]-?: T[P] };
  type Record<K extends keyof any, T> = { [P in K]: T };

  // Array interface
  interface Array<T> {
    push(...items: T[]): number;
    pop(): T | undefined;
    length: number;
    [n: number]: T;
  }

  interface ReadonlyArray<T> {
    readonly length: number;
    readonly [n: number]: T;
  }

  // Function and FunctionConstructor
  interface Function {
    // Function interface declaration
  }

  interface FunctionConstructor {
    (prototype?: any): any;
  }

  var Function: FunctionConstructor;

  // Date interface and constructor
  interface Date {
    toLocaleTimeString(): string;
    toLocaleString(): string;
    getTime(): number;
  }

  interface DateConstructor {
    new(): Date;
    now(): number;
  }

  var Date: DateConstructor;

  // JSON global
  interface JSON {
    parse(text: string, reviver?: (key: any, value: any) => any): any;
    stringify(value: any, replacer?: (key: string, value: any) => any, space?: string | number): string;
  }

  var JSON: JSON;

  // Console global
  interface Console {
    log(...args: any[]): void;
    error(...args: any[]): void;
    warn(...args: any[]): void;
    info(...args: any[]): void;
  }

  var console: Console;
}

// Ensure these types are available globally
export type GlobalReturnType<T extends (...args: any) => any> = T extends (...args: any) => infer R ? R : any;
export {};
