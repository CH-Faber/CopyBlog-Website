import type { CapacitorConfig } from "@capacitor/cli"

const config: CapacitorConfig = {
  appId: "top.faberhu.agenda",
  appName: "一个闪念",
  webDir: "dist",
  server: {
    hostname: "localhost",
    androidScheme: "https",
  },
  android: {
    allowMixedContent: false,
  },
  plugins: {
    LocalNotifications: {
      smallIcon: "ic_stat_agenda",
      iconColor: "#16A34A",
    },
  },
}

export default config
