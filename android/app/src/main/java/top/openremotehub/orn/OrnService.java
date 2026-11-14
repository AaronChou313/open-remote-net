package top.openremotehub.orn;

import android.app.*;
import android.content.*;
import android.net.*;
import android.os.*;
import java.io.File;
import java.util.concurrent.*;
import top.openremotehub.orn.core.mobile.Mobile;

/** Owns the Go runtime while the Activity or RustDesk is in the foreground. */
public final class OrnService extends Service {
    static volatile String error = "";
    private final ExecutorService worker = Executors.newSingleThreadExecutor();
    private ConnectivityManager connectivity;
    private ConnectivityManager.NetworkCallback callback;
    private final Handler main = new Handler(Looper.getMainLooper());
    private final Runnable rebind = () -> worker.execute(() -> {
        try { Mobile.rebind(); } catch (Exception e) { error = "网络切换后正在重连"; }
    });

    @Override public void onCreate() {
        super.onCreate();
        go.Seq.setContext(getApplicationContext());
        NotificationManager manager = getSystemService(NotificationManager.class);
        manager.createNotificationChannel(new NotificationChannel("orn", "ORN 连接", NotificationManager.IMPORTANCE_LOW));
        PendingIntent open = PendingIntent.getActivity(this,0,new Intent(this,MainActivity.class),PendingIntent.FLAG_IMMUTABLE);
        Notification notification = new Notification.Builder(this,"orn")
            .setSmallIcon(R.drawable.ic_orn).setContentTitle("OpenRemoteHub · ORN")
            .setContentText("设备网络正在运行").setContentIntent(open).setOngoing(true).build();
        startForeground(1,notification);
        worker.execute(() -> {
            try { Mobile.start(new File(getFilesDir(),"orn").getAbsolutePath()); error = ""; }
            catch (Exception e) { error = "ORN 启动失败，请检查本地端口或重新打开应用"; }
        });
        connectivity = getSystemService(ConnectivityManager.class);
        callback = new ConnectivityManager.NetworkCallback() {
            @Override public void onAvailable(Network network) { scheduleRebind(); }
            @Override public void onLost(Network network) { scheduleRebind(); }
            @Override public void onLinkPropertiesChanged(Network network,LinkProperties properties) { scheduleRebind(); }
        };
        connectivity.registerDefaultNetworkCallback(callback);
    }
    private void scheduleRebind() {
        main.removeCallbacks(rebind);
        main.postDelayed(rebind,1500);
    }
    @Override public int onStartCommand(Intent intent,int flags,int id) { return START_STICKY; }
    @Override public IBinder onBind(Intent intent) { return null; }
    @Override public void onDestroy() {
        connectivity.unregisterNetworkCallback(callback);
        main.removeCallbacks(rebind);
        worker.execute(() -> { try { Mobile.stop(); } catch (Exception ignored) {} });
        worker.shutdown();
        super.onDestroy();
    }
}
