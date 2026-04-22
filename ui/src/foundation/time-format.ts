export function shortDateTime(iso: string): string {
  const d = new Date(iso);
  const month = d.getMonth() + 1;
  const day = d.getDate();
  let hours = d.getHours();
  const minutes = d.getMinutes();
  const ampm = hours >= 12 ? "p" : "a";
  hours = hours % 12 || 12;
  return `${month}/${day} ${hours}:${minutes.toString().padStart(2, "0")}${ampm}`;
}
