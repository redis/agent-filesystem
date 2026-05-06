import type { ComponentType, SVGProps } from "react";
import type { MonochromeIconProps } from "@redis-ui/icons";
import { useTheme } from "@redis-ui/styles";
import {
  Bell,
  BookOpen,
  Bot,
  CloudDownload,
  Database,
  Folders,
  Laptop,
  LifeBuoy,
  PieChart,
  Plug,
  RefreshCw,
  Shield,
  Sparkles,
} from "lucide-react";

type LucideIcon = ComponentType<SVGProps<SVGSVGElement> & { size?: number | string; strokeWidth?: number }>;

// makeLucideIcon adapts a lucide-react icon to the Redis-UI MonochromeIconProps
// contract the sidebar expects (size tokens like "L", theme-aware color).
// Keeping a single wrapper guarantees every sidebar icon renders at the same
// resolved pixel size and stroke weight — lucide icons ship at varying default
// viewBoxes otherwise.
function makeLucideIcon(Icon: LucideIcon, defaultLabel: string) {
  return function WrappedIcon({
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
      theme.core.icon.size[size] ||
      theme.core.icon.size.L ||
      20;
    const colorValue =
      customColor ||
      (color && theme.semantic.color.icon[color]) ||
      "currentColor";

    return (
      <Icon
        size={sizeValue}
        color={colorValue}
        strokeWidth={1.75}
        aria-label={title ?? defaultLabel}
        {...rest}
      />
    );
  };
}

export const PieChartIcon = makeLucideIcon(PieChart, "Pie chart");
export const FoldersIcon = makeLucideIcon(Folders, "Folders");
export const BotIcon = makeLucideIcon(Bot, "Bot");
export const DatabaseIcon = makeLucideIcon(Database, "Database");
export const LaptopIcon = makeLucideIcon(Laptop, "Laptop");
export const BellIcon = makeLucideIcon(Bell, "Bell");
export const BookOpenIcon = makeLucideIcon(BookOpen, "Book");
export const CloudDownloadIcon = makeLucideIcon(CloudDownload, "Cloud download");
export const LifeBuoyIcon = makeLucideIcon(LifeBuoy, "Life buoy");
export const SparklesIcon = makeLucideIcon(Sparkles, "Sparkles");
export const PlugIcon = makeLucideIcon(Plug, "Plug");
export const RefreshCwIcon = makeLucideIcon(RefreshCw, "Refresh");
export const ShieldIcon = makeLucideIcon(Shield, "Shield");
