import type { MonochromeIconProps } from "@redis-ui/icons";
import { useTheme } from "@redislabsdev/redis-ui-styles";
import { PieChart } from "lucide-react";

export function PieChartIcon({
  size = "L",
  customSize,
  color,
  customColor,
  title,
  ...rest
}: MonochromeIconProps) {
  const theme = useTheme();
  const sizeValue =
    customSize ||
    theme?.core.icon.size[size] ||
    theme?.core.icon.size.L ||
    20;
  const colorValue =
    customColor ||
    (color && theme?.semantic.color.icon[color]) ||
    "currentColor";

  return (
    <PieChart
      size={sizeValue}
      color={colorValue}
      strokeWidth={1.75}
      aria-label={title ?? "Pie chart"}
      {...rest}
    />
  );
}
