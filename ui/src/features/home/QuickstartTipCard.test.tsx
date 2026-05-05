import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import {
  QUICKSTART_TIP_DISMISSED_KEY,
  QuickstartTipCard,
} from "./QuickstartTipCard";

describe("QuickstartTipCard", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  afterEach(() => {
    cleanup();
  });

  test("opens getting started from the inline preview", () => {
    const onOpen = vi.fn();

    render(<QuickstartTipCard onOpen={onOpen} />);

    fireEvent.click(screen.getByRole("button", { name: "Open Getting Started" }));

    expect(onOpen).toHaveBeenCalledTimes(1);
  });

  test("persists dismissal in localStorage", () => {
    render(<QuickstartTipCard onOpen={() => undefined} />);

    fireEvent.click(
      screen.getByRole("button", { name: "Dismiss Getting Started tip" }),
    );

    expect(
      screen.queryByRole("note", { name: "Getting Started tip" }),
    ).toBeNull();
    expect(window.localStorage.getItem(QUICKSTART_TIP_DISMISSED_KEY)).toBe("1");
  });

  test("stays hidden after being dismissed", () => {
    window.localStorage.setItem(QUICKSTART_TIP_DISMISSED_KEY, "1");

    render(<QuickstartTipCard onOpen={() => undefined} />);

    expect(
      screen.queryByRole("note", { name: "Getting Started tip" }),
    ).toBeNull();
  });
});
