"use client";

import { Bounce, ToastContainer } from "react-toastify";

/** Mounted once in the root layout; toasts survive client-side navigation (e.g. logout → /login). */
export function Toaster() {
  return (
    <ToastContainer
      position="top-right"
      autoClose={4000}
      newestOnTop
      closeOnClick
      pauseOnHover
      pauseOnFocusLoss
      draggable={false}
      limit={4}
      transition={Bounce}
      theme="light"
    />
  );
}
