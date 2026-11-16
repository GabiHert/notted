import "@testing-library/jest-dom/vitest";

// jsdom has no canvas: a 2D context is null (the live wave then draws nothing) instead of a "not implemented" error.
HTMLCanvasElement.prototype.getContext = (() => null) as unknown as HTMLCanvasElement["getContext"];
