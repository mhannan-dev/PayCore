"use client";

import { toast, type ToastOptions } from "react-toastify";

// One place for toast notifications so components never import the toast library
// directly. Use toasts for the outcome of an action (saved, signed out, exported);
// keep form validation errors inline next to their fields.
export const notify = {
  success: (message: string, options?: ToastOptions) => toast.success(message, options),
  error: (message: string, options?: ToastOptions) => toast.error(message, { autoClose: 7000, ...options }),
  info: (message: string, options?: ToastOptions) => toast.info(message, options),
  warning: (message: string, options?: ToastOptions) => toast.warning(message, { autoClose: 9000, ...options }),
};
