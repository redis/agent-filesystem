import type { MonochromeIconProps } from "@redis-ui/icons";
import { useTheme } from "@redislabsdev/redis-ui-styles";
import { Bot } from "lucide-react";

export function BotIcon({
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
    <Bot
      size={sizeValue}
      color={colorValue}
      strokeWidth={1.75}
      aria-label={title ?? "Bot"}
      {...rest}
    />
  );
}
