// Generic sort comparator used by table components.
// Numbers compare arithmetically; everything else compares as locale strings.
export function compareValues(
  left: string | number,
  right: string | number,
  direction: "asc" | "desc",
) {
  const result =
    typeof left === "number" && typeof right === "number"
      ? left - right
      : String(left).localeCompare(String(right));
  return direction === "asc" ? result : result * -1;
}
