import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, beforeEach } from "vitest";
import { ColorModeProvider, useColorMode } from "./theme-context";

function ThemeHarness() {
  const { colorMode, toggleColorMode } = useColorMode();

  return (
    <button type="button" onClick={toggleColorMode}>
      {colorMode}
    </button>
  );
}

function renderThemeHarness() {
  render(
    <ColorModeProvider>
      {() => <ThemeHarness />}
    </ColorModeProvider>,
  );
}

describe("ColorModeProvider", () => {
  beforeEach(() => {
    window.localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
  });

  it("reads the saved color mode and writes updates back to local storage", () => {
    window.localStorage.setItem("afs_color_mode", "dark");

    renderThemeHarness();

    expect(screen.getByRole("button", { name: "dark" })).toBeInTheDocument();
    expect(document.documentElement).toHaveAttribute("data-theme", "dark");

    fireEvent.click(screen.getByRole("button", { name: "dark" }));

    expect(screen.getByRole("button", { name: "light" })).toBeInTheDocument();
    expect(window.localStorage.getItem("afs_color_mode")).toBe("light");
    expect(document.documentElement).toHaveAttribute("data-theme", "light");
  });
});
