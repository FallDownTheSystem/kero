import { defineMiddleware } from "void";

// Umami analytics, only on the built site so dev visits aren't counted.
export default defineMiddleware(async (c, next) => {
  if (import.meta.env.PROD) {
    c.set("headDefaults", {
      script: [
        {
          defer: "",
          src: "https://u.egoist.dev/script.js",
          "data-website-id": "03d2a445-d03b-4823-921c-e6285693444e",
        },
      ],
    });
  }
  await next();
});
