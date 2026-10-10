const paths = {
  home: "M3 11.5 12 4l9 7.5M5 10v10h5v-6h4v6h5V10",
  box: "M21 8 12 3 3 8v8l9 5 9-5V8ZM3 8l9 5 9-5M12 13v8",
  scan: "M4 8V5a1 1 0 0 1 1-1h3M16 4h3a1 1 0 0 1 1 1v3M20 16v3a1 1 0 0 1-1 1h-3M8 20H5a1 1 0 0 1-1-1v-3M4 12h16",
  menu: "M4 7h16M4 12h16M4 17h16",
  search: "M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14ZM20 20l-4-4",
  filter: "M4 6h16M7 12h10M10 18h4",
  copy: "M9 9h10v11H9zM5 15V4h10",
  plus: "M12 5v14M5 12h14",
  minus: "M5 12h14",
  x: "M6 6l12 12M18 6 6 18",
  chevron: "M9 6l6 6-6 6",
  check: "M5 12.5 10 17 19 7",
  edit: "M4 20h4L19 9l-4-4L4 16v4ZM13 7l4 4",
  trash: "M5 7h14M10 7V4h4v3M7 7l1 13h8l1-13",
  logout: "M10 5H5v14h5M15 8l4 4-4 4M19 12H9",
} as const;

export type IconName = keyof typeof paths;

export function Icon({ name, className = "size-5" }: { name: IconName; className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className={className}
    >
      <path d={paths[name]} />
    </svg>
  );
}
