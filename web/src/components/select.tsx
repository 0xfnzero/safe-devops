"use client";

import * as Primitive from "@radix-ui/react-select";
import { Check, ChevronDown, ChevronUp } from "lucide-react";
import type { ComponentProps } from "react";

// Keep keyboard navigation, typeahead, form integration and focus restoration
// in the accessible primitive; every screen shares the same visual treatment.
export function Select({
  children,
  id,
  "aria-label": ariaLabel,
  ...props
}: ComponentProps<typeof Primitive.Root> & {
  id?: string;
  "aria-label"?: string;
}) {
  return (
    <Primitive.Root {...props}>
      <Primitive.Trigger
        id={id}
        aria-label={ariaLabel}
        className="select-trigger"
      >
        <Primitive.Value />
        <Primitive.Icon className="select-chevron">
          <ChevronDown size={15} />
        </Primitive.Icon>
      </Primitive.Trigger>
      <Primitive.Portal>
        <Primitive.Content
          className="select-menu"
          position="popper"
          sideOffset={6}
          collisionPadding={12}
          onEscapeKeyDown={(event) => event.stopPropagation()}
        >
          <Primitive.ScrollUpButton className="select-scroll">
            <ChevronUp size={14} />
          </Primitive.ScrollUpButton>
          <Primitive.Viewport className="select-viewport">
            {children}
          </Primitive.Viewport>
          <Primitive.ScrollDownButton className="select-scroll">
            <ChevronDown size={14} />
          </Primitive.ScrollDownButton>
        </Primitive.Content>
      </Primitive.Portal>
    </Primitive.Root>
  );
}

export function SelectItem({
  children,
  ...props
}: ComponentProps<typeof Primitive.Item>) {
  return (
    <Primitive.Item {...props} className="select-item">
      <Primitive.ItemText>{children}</Primitive.ItemText>
      <Primitive.ItemIndicator className="select-check">
        <Check size={14} />
      </Primitive.ItemIndicator>
    </Primitive.Item>
  );
}
