"use client";

import { createElement, forwardRef, type ImgHTMLAttributes } from "react";

const PlainImage = forwardRef<HTMLImageElement, ImgHTMLAttributes<HTMLImageElement>>(
  function PlainImage(props, ref) {
    return createElement("img", { ...props, ref });
  },
);

export default PlainImage;
