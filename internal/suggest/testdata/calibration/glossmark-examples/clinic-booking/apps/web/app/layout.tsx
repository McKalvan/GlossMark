import type { Metadata } from "next";
import type { ReactNode } from "react";
import { NavBar } from "@/components/NavBar";
import "./globals.css";

export const metadata: Metadata = {
  title: "Larkspur Clinics",
  description: "Book and manage appointments at Larkspur Clinics",
};

// @gloss implements .PatientSite conf=0.9
export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <body>
        <NavBar />
        <main className="container">{children}</main>
        <footer className="footer">Larkspur Clinics · Call 0800 555 0134 for urgent care</footer>
      </body>
    </html>
  );
}
